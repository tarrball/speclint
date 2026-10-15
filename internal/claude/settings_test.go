package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatcherCovers(t *testing.T) {
	for _, c := range []struct {
		matcher, tool string
		want          bool
	}{{"", "Edit", true}, {"*", "Write", true}, {"Write|Edit|MultiEdit", "Edit", true}, {"Write", "MultiEdit", false}, {"Bash", "Write", false}} {
		if got := MatcherCovers(c.matcher, c.tool); got != c.want {
			t.Errorf("MatcherCovers(%q, %q) = %v", c.matcher, c.tool, got)
		}
	}
}

func TestAddHookKeepsExistingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	content := `{"model":"opus","hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo hi"}]}]}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if added, err := AddHook(path); err != nil || !added {
		t.Fatalf("AddHook = %v, %v", added, err)
	}
	if added, _ := AddHook(path); added {
		t.Fatal("second AddHook should be a no-op")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"model": "opus"`, `"echo hi"`, `"specval hook"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("settings lost %s:\n%s", want, b)
		}
	}
}
