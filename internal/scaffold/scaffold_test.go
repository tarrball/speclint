package scaffold

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tarrball/specval/internal/claude"
	"github.com/tarrball/specval/internal/config"
)

func TestInitCreatesAProject(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := Init(dir, &out); err != nil {
		t.Fatal(err)
	}
	want := `created  .specval.yaml
created  .specval/data/services.yaml
created  .specval/policy/project.rego
created  .claude/skills/spec/SKILL.md
hooked   .claude/settings.json
`
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
	for _, p := range []string{config.FileName, ".specval/data/services.yaml", ".specval/policy/project.rego", claude.SkillPath, claude.SettingsPath} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "specs")); err != nil || !info.IsDir() {
		t.Errorf("specs directory: %v", err)
	}
	// The new configuration gives the same values as the defaults.
	cfg, err := config.Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SpecsDir != "specs" || len(cfg.Policy) != 1 || len(cfg.Data) != 1 {
		t.Errorf("config = %+v", cfg)
	}
}

func TestInitDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	services := filepath.Join(dir, ".specval", "data", "services.yaml")
	if err := os.WriteFile(services, []byte("services: {X: {layer: manager}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Init(dir, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "created") || strings.Contains(out.String(), "hooked") {
		t.Errorf("second Init changed files:\n%s", out.String())
	}
	b, err := os.ReadFile(services)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "X:") {
		t.Error("second Init overwrote services.yaml")
	}
}

func TestInitUsesTheConfiguredSpecsDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte("specs_dir: docs/changes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "changes")); err != nil {
		t.Errorf("configured specs directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "specs")); !os.IsNotExist(err) {
		t.Errorf("Init made the default specs directory: %v", err)
	}
}

func TestInitWritesNothingIfTheSettingsAreNotValid(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, claude.SettingsPath)
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Init(dir, &out); err == nil || !strings.Contains(err.Error(), "is not valid JSON") {
		t.Fatalf("Init = %v, want an invalid JSON error", err)
	}
	if out.Len() != 0 {
		t.Errorf("Init wrote output:\n%s", out.String())
	}
	for _, p := range []string{config.FileName, ".specval", "specs", claude.SkillPath} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("Init made %s", p)
		}
	}
}

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{
		"ORD-142":   true,
		"a.b_c-1":   true,
		"1":         true,
		"":          false,
		"-ORD":      false,
		".hidden":   false,
		"ORD 142":   false,
		"ORD/142":   false,
		"../escape": false,
	} {
		if got := ValidID(id); got != want {
			t.Errorf("ValidID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestSpec(t *testing.T) {
	cfg := &config.Config{Root: t.TempDir(), SpecsDir: "specs"}
	path, err := Spec(cfg, "BUG-7", "bugfix", `Fix the "total" field`)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cfg.Root, "specs", "BUG-7.yaml"); path != want {
		t.Errorf("path = %s, want %s", path, want)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Specval int    `yaml:"specval"`
		ID      string `yaml:"id"`
		Kind    string `yaml:"kind"`
		Title   string `yaml:"title"`
	}
	if err := yaml.Unmarshal(b, &spec); err != nil {
		t.Fatalf("the new spec is not valid YAML: %v\n%s", err, b)
	}
	if spec.Specval != 1 || spec.ID != "BUG-7" || spec.Kind != "bugfix" || spec.Title != `Fix the "total" field` {
		t.Errorf("spec = %+v", spec)
	}

	if _, err := Spec(cfg, "BUG-7", "bugfix", ""); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second Spec: err = %v, want already exists", err)
	}
}

func TestSpecTitleRoundTrips(t *testing.T) {
	cfg := &config.Config{Root: t.TempDir(), SpecsDir: "specs"}
	for i, title := range []string{
		"",
		`Fix the "total" field`,
		`C:\path\to\file`,
		"key: value # not a comment",
		"two\nlines",
		"tab\there",
		"bell\a and nul\x00",
		"émoji 🎉 and \u2028",
		"'single' quotes",
		"- not a list",
	} {
		path, err := Spec(cfg, fmt.Sprintf("T-%d", i), "chore", title)
		if err != nil {
			t.Errorf("%q: %v", title, err)
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var spec struct {
			Title string `yaml:"title"`
		}
		if err := yaml.Unmarshal(b, &spec); err != nil {
			t.Errorf("%q: the spec is not valid YAML: %v", title, err)
			continue
		}
		if spec.Title != title {
			t.Errorf("title %q was read back as %q", title, spec.Title)
		}
	}
}

func TestSpecRejectsATitleThatIsNotUTF8(t *testing.T) {
	cfg := &config.Config{Root: t.TempDir(), SpecsDir: "specs"}
	if _, err := Spec(cfg, "T-1", "chore", "bad \xff byte"); err == nil {
		t.Error("Spec accepted a title that is not valid UTF-8")
	}
	if _, err := os.Stat(filepath.Join(cfg.Root, "specs", "T-1.yaml")); !os.IsNotExist(err) {
		t.Errorf("Spec wrote a file: %v", err)
	}
}
