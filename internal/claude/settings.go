// Package claude reads and changes the Claude Code files that specval uses:
// the settings file that holds the hook, the skill, and the hook event.
package claude

import (
	"bytes"
	_ "embed" // For the go:embed directives.
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Paths of the Claude Code files, relative to a project or home directory.
const (
	SkillPath         = ".claude/skills/spec/SKILL.md"
	SettingsPath      = ".claude/settings.json"
	LocalSettingsPath = ".claude/settings.local.json"
)

var (
	//go:embed templates/SKILL.md
	skill string

	//go:embed templates/settings.json
	settingsTemplate []byte
)

// Skill returns the text of the skill that sv init writes. specsDir is
// the specs directory of the project, as .specval.yaml gives it. Only the
// Markdown body uses it, so that a name with YAML syntax cannot make the
// frontmatter invalid.
func Skill(specsDir string) string {
	return strings.ReplaceAll(skill, "{{SPECS_DIR}}", filepath.ToSlash(specsDir))
}

type settingsFile struct {
	DisableAllHooks bool `json:"disableAllHooks"`
	Hooks           map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

// HookStatus tells what a settings file contains about the sv hook.
type HookStatus struct {
	// Exists is true if the settings file exists.
	Exists bool
	// Found is true if a PostToolUse hook runs `sv hook`.
	Found bool
	// Disabled is true if the file sets disableAllHooks.
	Disabled bool
	// Matcher is the tool matcher of the hook, if Found is true.
	Matcher string
	// Old is true if a PostToolUse hook runs `specval hook`. The command had
	// that name before it was sv.
	Old bool
}

// InspectSettings reads a Claude Code settings file and tells whether it runs
// `sv hook` on PostToolUse. A file that does not exist is not an error.
func InspectSettings(path string) (HookStatus, error) {
	var st HookStatus
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	} else if err != nil {
		return st, err
	}
	st.Exists = true
	var s settingsFile
	if err := json.Unmarshal(b, &s); err != nil {
		// A field with a value of the wrong type is valid JSON, so say which
		// field it is, not that the JSON is not valid.
		if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			// An empty field is the top level, for example [] or true.
			if typeErr.Field == "" {
				return st, fmt.Errorf("%s: the settings must be a JSON object", path)
			}
			return st, fmt.Errorf("%s: the field %q has the wrong type", path, typeErr.Field)
		}
		return st, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	st.Disabled = s.DisableAllHooks
	// Read each hook, because an old hook can come after the sv hook.
	for _, entry := range s.Hooks["PostToolUse"] {
		for _, h := range entry.Hooks {
			if strings.Contains(h.Command, "sv hook") && !st.Found {
				st.Found, st.Matcher = true, entry.Matcher
			}
			st.Old = st.Old || strings.Contains(h.Command, "specval hook")
		}
	}
	return st, nil
}

// AddHook adds the specval PostToolUse hook to a settings file. It keeps the
// other settings, and creates the file if it does not exist. It returns false
// if the hook was already there.
//
// AddHook changes only the PostToolUse list. It keeps the order of the keys,
// the text of each other value, and the indent of the file.
func AddHook(path string) (bool, error) {
	st, err := InspectSettings(path)
	if err != nil || st.Found {
		return false, err
	}
	var out []byte
	if st.Exists {
		b, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		if out, err = addHook(b); err != nil {
			return false, fmt.Errorf("%s: %w", path, err)
		}
	} else {
		out = settingsTemplate
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}

// addHook returns the settings in b with the hooks of the template added to
// hooks.PostToolUse.
func addHook(b []byte) ([]byte, error) {
	var tmpl struct {
		Hooks struct {
			PostToolUse []json.RawMessage `json:"PostToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(settingsTemplate, &tmpl); err != nil {
		return nil, err
	}
	top, err := members(b)
	if err != nil {
		return nil, err
	}
	hooks, err := members(top.get("hooks"))
	if err != nil {
		return nil, err
	}
	var post []json.RawMessage
	if v := hooks.get("PostToolUse"); v != nil {
		if err := json.Unmarshal(v, &post); err != nil {
			return nil, err
		}
	}
	post = append(post, tmpl.Hooks.PostToolUse...)
	hooks = hooks.set("PostToolUse", encodeArray(post))
	obj, err := hooks.encode()
	if err != nil {
		return nil, err
	}
	top = top.set("hooks", obj)
	raw, err := top.encode()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", indentOf(b)); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// member is one key and its value in a JSON object. The value is the text
// from the file, so that it does not change.
type member struct {
	key   string
	value json.RawMessage
}

// object is a JSON object with its keys in the order of the file.
type object []member

// members reads a JSON object and keeps the order of its keys. A nil b and
// a JSON null are an empty object, as for encoding/json.
func members(b []byte) (object, error) {
	if b == nil || string(bytes.TrimSpace(b)) == "null" {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil {
		return nil, err
	} else if t != json.Delim('{') {
		return nil, errors.New("the settings must be a JSON object")
	}
	var obj object
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := t.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		obj = append(obj, member{key, v})
	}
	return obj, nil
}

// get returns the value of key, or nil. If the key is in the object more
// than one time, the last value counts, as in encoding/json.
func (o object) get(key string) json.RawMessage {
	for _, m := range slices.Backward(o) {
		if m.key == key {
			return m.value
		}
	}
	return nil
}

// set changes the last value of key, or adds the key at the end.
func (o object) set(key string, v json.RawMessage) object {
	for i, m := range slices.Backward(o) {
		if m.key == key {
			o[i].value = v
			return o
		}
	}
	return append(o, member{key, v})
}

func (o object) encode() (json.RawMessage, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := marshal(m.key)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(m.value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func encodeArray(items []json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, v := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(v)
	}
	buf.WriteByte(']')
	return buf.Bytes()
}

// marshal encodes v as JSON, and does not write &, <, and > as \u escapes.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// indentOf returns the indent of the first indented line in b, or two spaces.
func indentOf(b []byte) string {
	for line := range bytes.Lines(b) {
		trimmed := bytes.TrimLeft(line, " \t")
		if n := len(line) - len(trimmed); n > 0 && len(bytes.TrimSpace(trimmed)) > 0 {
			return string(line[:n])
		}
	}
	return "  "
}

// MatcherCovers reports whether a hook matcher fires for a tool name.
// An empty matcher or "*" matches every tool.
func MatcherCovers(matcher, tool string) bool {
	if matcher == "" || matcher == "*" {
		return true
	}
	re, err := regexp.Compile("^(?:" + matcher + ")$")
	return err == nil && re.MatchString(tool)
}
