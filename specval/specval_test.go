package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

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
	sort.Strings(ids)
	return ids
}

func TestDemoSpecs(t *testing.T) {
	cases := map[string]string{
		"ORD-142.yaml": "",
		"ORD-143.yaml": "ORD001 SV101 SV102 SV107 SV108 SV109",
		"ORD-144.yaml": "SV001 SV001 SV001 SV001 SV001 SV001",
	}
	for spec, want := range cases {
		if got := strings.Join(checkDemo(t, spec), " "); got != want {
			t.Errorf("%s: got [%s], want [%s]", spec, got, want)
		}
	}
}

func TestRuleWithoutIDIsRejected(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, configName), []byte("specs_dir: specs\npolicy: [p]\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "p"), 0o755)
	os.WriteFile(filepath.Join(dir, "p", "x.rego"), []byte("package specval.spec.x\n\ndeny contains \"nope\" if true\n"), 0o644)
	cfg, _ := findConfig(dir)
	if _, err := newEngine(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "custom.id") {
		t.Fatalf("want custom.id error, got %v", err)
	}
}
