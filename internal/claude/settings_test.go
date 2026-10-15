package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSkillRunsSpecvalContext(t *testing.T) {
	// doctor uses this text to tell that the skill is the specval skill.
	if !strings.Contains(Skill("specs"), "specval context") {
		t.Error("the skill does not tell Claude to run `specval context`")
	}
}

func TestInspectSettings(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name, content string
		want          HookStatus
	}{
		{"missing.json", "", HookStatus{}},
		{"empty.json", `{}`, HookStatus{Exists: true}},
		{"disabled.json", `{"disableAllHooks": true}`, HookStatus{Exists: true, Disabled: true}},
		{
			"other.json",
			`{"hooks": {"PostToolUse": [{"matcher": "Bash", "hooks": [{"command": "echo"}]}]}}`,
			HookStatus{Exists: true},
		},
		{
			"pretooluse.json",
			`{"hooks": {"PreToolUse": [{"matcher": "Write", "hooks": [{"command": "specval hook"}]}]}}`,
			HookStatus{Exists: true},
		},
		{
			"found.json",
			`{"hooks": {"PostToolUse": [{"matcher": "Bash", "hooks": [{"command": "echo"}]},
			  {"matcher": "Write|Edit", "hooks": [{"command": "/usr/local/bin/specval hook"}]}]}}`,
			HookStatus{Exists: true, Found: true, Matcher: "Write|Edit"},
		},
	} {
		path := filepath.Join(dir, c.name)
		if c.content != "" {
			writeFile(t, path, c.content)
		}
		got, err := InspectSettings(path)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestInspectSettingsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, "{not json")
	st, err := InspectSettings(path)
	if err == nil || !strings.Contains(err.Error(), path+" is not valid JSON") {
		t.Errorf("err = %v, want an invalid JSON error with the path", err)
	}
	if !st.Exists {
		t.Error("Exists = false, want true")
	}
}

func TestAddHookCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	if added, err := AddHook(path); err != nil || !added {
		t.Fatalf("AddHook = %v, %v", added, err)
	}
	st, err := InspectSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Found || st.Matcher != "Write|Edit|MultiEdit" {
		t.Errorf("after AddHook: %+v", st)
	}
	for _, tool := range []string{"Write", "Edit", "MultiEdit"} {
		if !MatcherCovers(st.Matcher, tool) {
			t.Errorf("the new hook does not fire on %s", tool)
		}
	}
}

func TestAddHookKeepsExistingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{"model":"opus","hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo hi"}]}],"Stop":[]}}`)
	if added, err := AddHook(path); err != nil || !added {
		t.Fatalf("AddHook = %v, %v", added, err)
	}
	if added, err := AddHook(path); err != nil || added {
		t.Fatalf("second AddHook = %v, %v; want false, nil", added, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Model string                      `json:"model"`
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Model != "opus" || len(s.Hooks["PostToolUse"]) != 2 || s.Hooks["Stop"] == nil {
		t.Errorf("settings after AddHook:\n%s", b)
	}
}

func TestAddHookInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, "{not json")
	if added, err := AddHook(path); err == nil || added {
		t.Errorf("AddHook = %v, %v; want false and an error", added, err)
	}
}

func TestMatcherCovers(t *testing.T) {
	for _, c := range []struct {
		matcher, tool string
		want          bool
	}{
		{"", "Edit", true},
		{"*", "Write", true},
		{"Write|Edit|MultiEdit", "Edit", true},
		{"Write|Edit", "MultiEdit", false},
		{"Write", "MultiEdit", false},
		{"Bash", "Write", false},
		{".*Edit", "MultiEdit", true},
		{"(", "Write", false}, // not a valid regular expression
	} {
		if got := MatcherCovers(c.matcher, c.tool); got != c.want {
			t.Errorf("MatcherCovers(%q, %q) = %v, want %v", c.matcher, c.tool, got, c.want)
		}
	}
}

func TestInspectSettingsWrongShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for content, field := range map[string]string{
		`{"hooks": []}`: "hooks",
		`{"hooks": {"PostToolUse": [{"matcher": 1}]}}`: "hooks.matcher", // Go leaves out map keys and list indexes.
		`{"disableAllHooks": "yes"}`:                   "disableAllHooks",
	} {
		writeFile(t, path, content)
		_, err := InspectSettings(path)
		want := path + `: the field "` + field + `" has the wrong type`
		if err == nil || err.Error() != want {
			t.Errorf("%s: err = %v, want %q", content, err, want)
		}
	}
}

func TestInspectSettingsThatIsNotAnObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, content := range []string{"[]", "true", `"x"`} {
		writeFile(t, path, content)
		_, err := InspectSettings(path)
		if want := path + ": the settings must be a JSON object"; err == nil || err.Error() != want {
			t.Errorf("%s: err = %v, want %q", content, err, want)
		}
	}
}

// The skill starts with YAML frontmatter. A specs directory with YAML syntax
// in its name must not make the frontmatter invalid.
func TestSkillFrontmatterIsValidYAML(t *testing.T) {
	for _, dir := range []string{"specs", "changes: draft", "a #b", `"quoted"`} {
		parts := strings.SplitN(Skill(dir), "---\n", 3)
		if len(parts) != 3 {
			t.Fatalf("%q: the skill has no frontmatter", dir)
		}
		var front struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &front); err != nil || front.Name != "spec" || front.Description == "" {
			t.Errorf("%q: frontmatter %+v, err %v", dir, front, err)
		}
		if !strings.Contains(parts[2], dir+"/<ticket-id>.yaml") {
			t.Errorf("%q: the body does not name the specs directory", dir)
		}
	}
}
