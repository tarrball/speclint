package scaffold

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
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

func TestProjectTemplateCompiles(t *testing.T) {
	// A comment line that starts with the word METADATA is an annotation for
	// OPA. Thus a change to the comments of the template can break it.
	if _, err := ast.ParseModuleWithOpts("project.rego", projectTemplate, ast.ParserOptions{ProcessAnnotation: true, RegoVersion: ast.RegoV1}); err != nil {
		t.Fatal(err)
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

func TestInitStopsOnAConfigurationThatIsNotValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte("specs_dir: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Init(dir, &out); err == nil {
		t.Fatalf("Init = nil, want the configuration error; output:\n%s", out.String())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("Init wrote files: %v", entries)
	}
}

func TestInitUsesTheConfiguredDataAndPolicyPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte("data: [registry.yaml]\npolicy: [rules]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Init(dir, &out); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"registry.yaml", filepath.Join("rules", "project.rego")} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".specval")); !os.IsNotExist(err) {
		t.Errorf("Init wrote .specval, which the configuration does not use")
	}
	if !strings.Contains(out.String(), "created  registry.yaml\n") {
		t.Errorf("output:\n%s", out.String())
	}
}

// initIn writes the given files to a new directory, runs Init there, and
// returns the directory and the output of Init.
func initIn(t *testing.T, files map[string]string) (dir, output string) {
	t.Helper()
	dir = t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := Init(dir, &out); err != nil {
		t.Fatalf("Init: %v\noutput:\n%s", err, out.String())
	}
	return dir, out.String()
}

func TestInitKeepsAnExistingRegistry(t *testing.T) {
	dir, out := initIn(t, map[string]string{
		config.FileName:      "data: [data]\n",
		"data/registry.yaml": "services:\n  OrderManager: { layer: manager }\n",
	})
	if _, err := os.Stat(filepath.Join(dir, "data", "services.yaml")); !os.IsNotExist(err) {
		t.Error("Init wrote data/services.yaml, which defines services a second time")
	}
	if !strings.Contains(out, "exists   data/registry.yaml (services)\n") {
		t.Errorf("output:\n%s", out)
	}
}

func TestInitWritesJSONForAJSONRegistry(t *testing.T) {
	dir, _ := initIn(t, map[string]string{config.FileName: "data: [registry.json]\n"})
	b, err := os.ReadFile(filepath.Join(dir, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("registry.json is not valid JSON: %v\n%s", err, b)
	}
	if _, ok := v["services"]; !ok {
		t.Errorf("registry.json has no services:\n%s", b)
	}
}

func TestInitLooksAtWhatAConfiguredPathIs(t *testing.T) {
	// An existing file without an extension is the registry file.
	dir, out := initIn(t, map[string]string{
		config.FileName: "data: [registry]\npolicy: [rules.rego]\n",
		"registry":      "services: {}\n",
		"rules.rego/":   "",
	})
	if !strings.Contains(out, "exists   registry (services)\n") {
		t.Errorf("output:\n%s", out)
	}
	// An existing directory with an extension in its name is a directory.
	if _, err := os.Stat(filepath.Join(dir, "rules.rego", "project.rego")); err != nil {
		t.Errorf("rules.rego/project.rego: %v", err)
	}
}

func TestInitWritesTheSpecsDirIntoTheSkill(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte("specs_dir: changes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, claude.SkillPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "changes/<ticket-id>.yaml") || strings.Contains(string(b), "specs/<ticket-id>") {
		t.Errorf("the skill does not use the configured specs_dir:\n%s", b)
	}
}

func TestSpecValidatesItsArguments(t *testing.T) {
	cfg := &config.Config{Root: t.TempDir(), SpecsDir: "specs"}
	for _, c := range []struct{ id, kind, want string }{
		{"../escape", "chore", `the id "../escape" is not valid`},
		{"A-1", "epic", `the kind "epic" is not valid; use one of: feature, bugfix, chore`},
	} {
		if _, err := Spec(cfg, c.id, c.kind, ""); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Spec(%q, %q): err = %v, want %q", c.id, c.kind, err, c.want)
		}
	}
	if entries, _ := os.ReadDir(cfg.Root); len(entries) != 0 {
		t.Errorf("Spec wrote files: %v", entries)
	}
}
