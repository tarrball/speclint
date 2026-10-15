package policy

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tarrball/specval/internal/config"
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
	cfg, err := config.Find("../../examples/refund")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	found, err := eng.CheckSpec(ctx, filepath.Join(cfg.Root, "specs", spec))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, f := range found {
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
	writeFile(t, filepath.Join(dir, config.FileName), "specs_dir: specs\npolicy: [p]\n")
	writeFile(t, filepath.Join(dir, "p", "x.rego"), "package specval.spec.x\n\ndeny contains \"nope\" if true\n")
	cfg, err := config.Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "custom.id") {
		t.Fatalf("want custom.id error, got %v", err)
	}
}
