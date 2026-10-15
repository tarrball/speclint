// Package claude reads and changes the Claude Code files that specval uses:
// the settings file that holds the hook, the skill, and the hook event.
package claude

import (
	_ "embed" // The templates are embedded.
	"encoding/json"
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

// Skill returns the text of the skill that specval init writes.
func Skill() string { return skill }

type settingsFile struct {
	DisableAllHooks bool `json:"disableAllHooks"`
	Hooks           map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

// HookStatus tells what a settings file contains about the specval hook.
type HookStatus struct {
	// Exists is true if the settings file exists.
	Exists bool
	// Found is true if a PostToolUse hook runs `specval hook`.
	Found bool
	// Disabled is true if the file sets disableAllHooks.
	Disabled bool
	// Matcher is the tool matcher of the hook, if Found is true.
	Matcher string
}

// InspectSettings reads a Claude Code settings file and tells whether it runs
// `specval hook` on PostToolUse. A file that does not exist is not an error.
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
		return st, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	st.Disabled = s.DisableAllHooks
	for _, entry := range s.Hooks["PostToolUse"] {
		for _, h := range entry.Hooks {
			if strings.Contains(h.Command, "specval hook") {
				st.Found, st.Matcher = true, entry.Matcher
				return st, nil
			}
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
