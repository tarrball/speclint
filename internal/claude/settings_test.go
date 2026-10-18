package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	if !strings.Contains(Skill("specs"), "sv context") {
		t.Error("the skill does not tell Claude to run `sv context`")
	}
}

func TestSkillTellsWhenToRunCheck(t *testing.T) {
	skill := Skill("specs")
	for _, want := range []string{
		"If you write the file in another way, for example with a shell command, run `sv check <file>`.",
		"set `status: done` in the spec. Then run `sv check`.",
		"write `calls: []` and put each changed service in `services.changed`",
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("the skill does not contain %q", want)
		}
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
			`{"hooks": {"PreToolUse": [{"matcher": "Write", "hooks": [{"command": "sv hook"}]}]}}`,
			HookStatus{Exists: true},
		},
		{
			"found.json",
			`{"hooks": {"PostToolUse": [{"matcher": "Bash", "hooks": [{"command": "echo"}]},
			  {"matcher": "Write|Edit", "hooks": [{"command": "/usr/local/bin/sv hook"}]}]}}`,
			HookStatus{Exists: true, Found: true, Matcher: "Write|Edit"},
		},
		{
			"current-then-old.json",
			`{"hooks": {"PostToolUse": [{"matcher": "Write", "hooks": [{"command": "sv hook"}]},
			  {"matcher": "Edit", "hooks": [{"command": "specval hook"}]}]}}`,
			HookStatus{Exists: true, Found: true, Matcher: "Write", Old: true},
		},
		{
			"old-then-current.json",
			`{"hooks": {"PostToolUse": [{"matcher": "Edit", "hooks": [{"command": "specval hook"}]},
			  {"matcher": "Write", "hooks": [{"command": "sv hook"}]}]}}`,
			HookStatus{Exists: true, Found: true, Matcher: "Write", Old: true},
		},
		{
			"old.json",
			`{"hooks": {"PostToolUse": [{"matcher": "Write", "hooks": [{"command": "specval hook"}]}]}}`,
			HookStatus{Exists: true, Old: true},
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

func TestAddHookKeepsTheFileAsItIs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	// The keys are not in lexical order, a command has &&, a number is
	// larger than a float64 can hold, and the indent is four spaces.
	writeFile(t, path, `{
    "model": "opus",
    "env": {"LIMIT": 12345678901234567890, "RATIO": 1.0},
    "hooks": {
        "Stop": [],
        "PostToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "a && b <c>"}]}]
    },
    "apiKeyHelper": "x"
}
`)
	if added, err := AddHook(path); err != nil || !added {
		t.Fatalf("AddHook = %v, %v", added, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{
    "model": "opus",
    "env": {
        "LIMIT": 12345678901234567890,
        "RATIO": 1.0
    },
    "hooks": {
        "Stop": [],
        "PostToolUse": [
            {
                "matcher": "Bash",
                "hooks": [
                    {
                        "type": "command",
                        "command": "a && b <c>"
                    }
                ]
            },
            {
                "matcher": "Write|Edit|MultiEdit",
                "hooks": [
                    {
                        "type": "command",
                        "command": ` + strconv.Quote(templateCommand(t)) + `
                    }
                ]
            }
        ]
    },
    "apiKeyHelper": "x"
}
`
	if got != want {
		t.Errorf("settings:\n%s\nwant:\n%s", got, want)
	}
}

func TestAddHookWithoutHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{"model": "opus"}`)
	if added, err := AddHook(path); err != nil || !added {
		t.Fatalf("AddHook = %v, %v", added, err)
	}
	st, err := InspectSettings(path)
	if err != nil || !st.Found {
		t.Errorf("InspectSettings = %+v, %v; want the hook", st, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), "{\n  \"model\": \"opus\",\n  \"hooks\": {") {
		t.Errorf("settings:\n%s", b)
	}
}

func TestAddHookWritesTheTemplateAsItIs(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	if _, err := AddHook(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, settingsTemplate) {
		t.Errorf("settings:\n%s\nwant the template:\n%s", b, settingsTemplate)
	}
}

func TestAddHookWithNullValues(t *testing.T) {
	// encoding/json reads null as an empty value, so InspectSettings accepts
	// these files. AddHook must add the hook to them.
	for _, settings := range []string{`null`, `{"hooks": null}`, `{"hooks": {"PostToolUse": null}}`} {
		path := filepath.Join(t.TempDir(), "settings.json")
		writeFile(t, path, settings)
		if added, err := AddHook(path); err != nil || !added {
			t.Errorf("%s: AddHook = %v, %v", settings, added, err)
			continue
		}
		if st, err := InspectSettings(path); err != nil || !st.Found {
			t.Errorf("%s: InspectSettings = %+v, %v; want the hook", settings, st, err)
		}
	}
}

func TestNeedsHookDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.json")
	if need, err := NeedsHook(missing); err != nil || !need {
		t.Errorf("NeedsHook(missing) = %v, %v", need, err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("NeedsHook made the file: %v", err)
	}
	other := filepath.Join(dir, "other.json")
	writeFile(t, other, `{"model": "opus"}`)
	if need, err := NeedsHook(other); err != nil || !need {
		t.Errorf("NeedsHook(other) = %v, %v", need, err)
	}
	if b, err := os.ReadFile(other); err != nil || string(b) != `{"model": "opus"}` {
		t.Errorf("NeedsHook changed the file: %q, %v", b, err)
	}
	if _, err := AddHook(other); err != nil {
		t.Fatal(err)
	}
	if need, err := NeedsHook(other); err != nil || need {
		t.Errorf("NeedsHook after AddHook = %v, %v", need, err)
	}
	// NeedsHook gives the errors of AddHook, for example a hooks field that is
	// not an object.
	wrong := filepath.Join(dir, "wrong.json")
	writeFile(t, wrong, `{"hooks": {"PostToolUse": {}}}`)
	_, addErr := AddHook(wrong)
	if _, err := NeedsHook(wrong); err == nil || addErr == nil || err.Error() != addErr.Error() {
		t.Errorf("NeedsHook error %v, AddHook error %v", err, addErr)
	}
}

// templateCommand returns the command of the hook in the settings template.
func templateCommand(t *testing.T) string {
	t.Helper()
	var s settingsFile
	if err := json.Unmarshal(settingsTemplate, &s); err != nil {
		t.Fatal(err)
	}
	post := s.Hooks["PostToolUse"]
	if len(post) != 1 || len(post[0].Hooks) != 1 {
		t.Fatalf("the template has %d PostToolUse entries; want one with one hook", len(post))
	}
	return post[0].Hooks[0].Command
}

func TestTemplateHookWithoutSvOnPath(t *testing.T) {
	// Claude Code runs the command with a shell, and the PATH that it gives to
	// hooks can be different from the PATH of the terminal.
	cmd := exec.Command("/bin/sh", "-c", templateCommand(t)) //nolint:gosec // The test runs the hook command of the embedded template.
	cmd.Env = []string{"PATH=" + t.TempDir()}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("the hook gave %v; want exit status 2, so that Claude Code sends stderr to Claude", err)
	}
	if !strings.Contains(stderr.String(), "sv is not on the PATH of Claude Code hooks") {
		t.Errorf("stderr: %q", stderr.String())
	}
}

func TestTemplateHookRunsSvHook(t *testing.T) {
	bin := t.TempDir()
	// The fake sv prints its arguments and the event that it reads on stdin.
	fake := "#!/bin/sh\nread -r event\necho \"$* $event\"\n"
	if err := os.WriteFile(filepath.Join(bin, "sv"), []byte(fake), 0o755); err != nil { // The fake binary must be executable.
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "-c", templateCommand(t)) //nolint:gosec // The test runs the hook command of the embedded template.
	cmd.Env = []string{"PATH=" + bin}
	cmd.Stdin = strings.NewReader(`{"tool_name":"Write"}` + "\n")
	out, err := cmd.Output()
	if err != nil || string(out) != "hook {\"tool_name\":\"Write\"}\n" {
		t.Errorf("the hook gave %q, %v; want sv hook with the event on stdin", out, err)
	}
}

func TestInspectSettingsFindsTheTemplateHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, string(settingsTemplate))
	if st, err := InspectSettings(path); err != nil || !st.Found || st.Matcher != "Write|Edit|MultiEdit" {
		t.Errorf("InspectSettings = %+v, %v; want the hook", st, err)
	}
}
