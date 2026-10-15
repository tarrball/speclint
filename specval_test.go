package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeFile creates path and its parent directories with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func checkDemo(t *testing.T, spec string) []string {
	t.Helper()
	ctx := context.Background()
	cfg, err := findConfig("examples/refund")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := newEngine(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	fs, err := eng.CheckSpec(ctx, filepath.Join(cfg.Root, "specs", spec))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, f := range fs {
		if f.Fix == "" || f.Line == 0 {
			t.Errorf("%s %s: every finding needs a fix and a line: %+v", spec, f.Rule, f)
		}
		ids = append(ids, f.Rule)
	}
	slices.Sort(ids)
	return ids
}

func TestDemoSpecs(t *testing.T) {
	cases := map[string][]string{
		"ORD-142.yaml": nil,
		"ORD-143.yaml": {"ORD001", "SV101", "SV102", "SV107", "SV108", "SV109"},
		"ORD-144.yaml": slices.Repeat([]string{"SV001"}, 6),
	}
	for spec, want := range cases {
		if got := checkDemo(t, spec); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", spec, got, want)
		}
	}
}

func TestRuleWithoutIDIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, configName), "specs_dir: specs\npolicy: [p]\n")
	writeFile(t, filepath.Join(dir, "p", "x.rego"), "package specval.spec.x\n\ndeny contains \"nope\" if true\n")
	cfg, err := findConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newEngine(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "custom.id") {
		t.Fatalf("want custom.id error, got %v", err)
	}
}

func TestMatcherCovers(t *testing.T) {
	for _, c := range []struct {
		matcher, tool string
		want          bool
	}{{"", "Edit", true}, {"*", "Write", true}, {"Write|Edit|MultiEdit", "Edit", true}, {"Write", "MultiEdit", false}, {"Bash", "Write", false}} {
		if got := matcherCovers(c.matcher, c.tool); got != c.want {
			t.Errorf("matcherCovers(%q, %q) = %v", c.matcher, c.tool, got)
		}
	}
}

func TestAddHookKeepsExistingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{"model":"opus","hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo hi"}]}]}}`)
	if added, err := addHook(path); err != nil || !added {
		t.Fatalf("addHook = %v, %v", added, err)
	}
	if added, _ := addHook(path); added {
		t.Fatal("second addHook should be a no-op")
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
