// Package claude reads and changes the Claude Code files that specval uses:
// the settings file that holds the hook, the skill, and the hook event.
package claude

import (
	_ "embed" // For the go:embed directives.
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
func AddHook(path string) (bool, error) {
	st, err := InspectSettings(path)
	if err != nil || st.Found {
		return false, err
	}
	s := map[string]any{}
	if st.Exists {
		b, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		if err := json.Unmarshal(b, &s); err != nil {
			return false, err
		}
	}
	var tmpl struct {
		Hooks struct {
			PostToolUse []any `json:"PostToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(settingsTemplate, &tmpl); err != nil {
		return false, err
	}
	hooks, _ := s["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		s["hooks"] = hooks
	}
	post, _ := hooks["PostToolUse"].([]any)
	hooks["PostToolUse"] = append(post, tmpl.Hooks.PostToolUse...)
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o644)
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
