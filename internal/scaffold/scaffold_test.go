package scaffold

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
	"go.yaml.in/yaml/v3"

	"github.com/tarrball/specval/internal/claude"
	"github.com/tarrball/specval/internal/config"
	"github.com/tarrball/specval/internal/copilot"
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
created  .github/instructions/specval.instructions.md
hooked   .claude/settings.json

Add this line to CLAUDE.md, so that Claude uses the spec skill for each change:
  ` + claude.MemoryLine + `
`
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
	for _, p := range []string{config.FileName, ".specval/data/services.yaml", ".specval/policy/project.rego", claude.SkillPath, copilot.InstructionsPath, claude.SettingsPath} {
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
	instructions := filepath.Join(dir, copilot.InstructionsPath)
	if err := os.WriteFile(instructions, []byte("Our own review rules.\n"), 0o644); err != nil {
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
	if b, err := os.ReadFile(instructions); err != nil || string(b) != "Our own review rules.\n" {
		t.Errorf("second Init changed the Copilot instructions: %q, %v", b, err)
	}
}

func TestInitWritesTheCopilotInstructions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte("specs_dir: changes\ndata: [arch/registry.yaml]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, copilot.InstructionsPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"`changes/<id>.yaml`", "`arch/registry.yaml`", "Spec deviations"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the instructions do not contain %q:\n%s", want, b)
		}
	}
}

func TestInitPrintsTheMemoryLine(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		hint  bool
	}{
		{"no CLAUDE.md", nil, true},
		{"CLAUDE.md without specval", map[string]string{"CLAUDE.md": "# Notes\n"}, true},
		{"CLAUDE.md with specval", map[string]string{"CLAUDE.md": "Use specval.\n"}, false},
		{".claude/CLAUDE.md with specval", map[string]string{"CLAUDE.md": "# Notes\n", ".claude/CLAUDE.md": "Use specval.\n"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for p, content := range c.files {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, p), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			if err := Init(dir, &out); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(out.String(), claude.MemoryLine); got != c.hint {
				t.Errorf("hint = %v, want %v:\n%s", got, c.hint, out.String())
			}
			// Init does not write CLAUDE.md.
			b, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
			if want, ok := c.files["CLAUDE.md"]; ok != (err == nil) || string(b) != want {
				t.Errorf("CLAUDE.md = %q, %v; want %q", b, err, want)
			}
		})
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
	path, err := writeSpec(cfg, "BUG-7", "bugfix", `Fix the "total" field`)
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

	if _, err := writeSpec(cfg, "BUG-7", "bugfix", ""); !errors.Is(err, fs.ErrExist) {
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
		path, err := writeSpec(cfg, fmt.Sprintf("T-%d", i), "chore", title)
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
	if _, err := writeSpec(cfg, "T-1", "chore", "bad \xff byte"); err == nil {
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

func TestSpecFindsTheProject(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, config.FileName), []byte("specs_dir: plans\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// The path is relative to the directory that Spec gets.
	path, err := Spec(sub, "A-1", "chore", "")
	if want := filepath.Join("..", "plans", "A-1.yaml"); err != nil || path != want {
		t.Errorf("Spec = %q, %v; want %q", path, err, want)
	}
	if _, err := os.Stat(filepath.Join(root, "plans", "A-1.yaml")); err != nil {
		t.Error(err)
	}
	path, err = Spec(root, "A-2", "chore", "")
	if want := filepath.Join("plans", "A-2.yaml"); err != nil || path != want {
		t.Errorf("Spec = %q, %v; want %q", path, err, want)
	}
	_, err = Spec(root, "A-2", "chore", "")
	if want := filepath.Join("plans", "A-2.yaml") + " already exists; edit it, or use another id"; err == nil || err.Error() != want {
		t.Errorf("second Spec: err = %v, want %q", err, want)
	}
	if _, err := Spec(t.TempDir(), "A-1", "chore", ""); !errors.Is(err, config.ErrNotFound) {
		t.Errorf("outside a project: err = %v, want config.ErrNotFound", err)
	}
}

func TestSpecValidatesItsArguments(t *testing.T) {
	cfg := &config.Config{Root: t.TempDir(), SpecsDir: "specs"}
	for _, c := range []struct{ id, kind, want string }{
		{"../escape", "chore", `the id "../escape" is not valid`},
		{"A-1", "epic", `the kind "epic" is not valid; use one of: feature, bugfix, chore`},
	} {
		if _, err := writeSpec(cfg, c.id, c.kind, ""); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Spec(%q, %q): err = %v, want %q", c.id, c.kind, err, c.want)
		}
	}
	if entries, _ := os.ReadDir(cfg.Root); len(entries) != 0 {
		t.Errorf("Spec wrote files: %v", entries)
	}
}

// A dangling symbolic link at the destination is a file that exists. Spec must
// not follow the link and create its target. A check before the write, then a
// write, would do that.
func TestSpecDoesNotFollowADanglingLink(t *testing.T) {
	cfg := &config.Config{Root: t.TempDir(), SpecsDir: "specs"}
	target := filepath.Join(t.TempDir(), "elsewhere.yaml")
	link := filepath.Join(cfg.Root, "specs", "A-1.yaml")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := writeSpec(cfg, "A-1", "chore", ""); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Spec: err = %v, want already exists", err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link changed: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("Spec created the target of the link: %v", err)
	}
}

// files returns the paths of the files and directories in dir.
func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if p != dir {
			out = append(out, relPath(dir, p))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := DryRun(dir, &out); err != nil {
		t.Fatal(err)
	}
	want := `create   .specval.yaml
create   .specval/data/services.yaml
create   .specval/policy/project.rego
create   .claude/skills/spec/SKILL.md
create   .github/instructions/specval.instructions.md
hook     .claude/settings.json

This is a dry run. sv init wrote nothing.

Add this line to CLAUDE.md, so that Claude uses the spec skill for each change:
  ` + claude.MemoryLine + `
`
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
	if got := files(t, dir); len(got) != 0 {
		t.Errorf("DryRun wrote %v", got)
	}
}

func TestDryRunAfterInit(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	before := files(t, dir)
	var out bytes.Buffer
	if err := DryRun(dir, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "create ") || !strings.Contains(out.String(), "exists   .claude/settings.json (sv hook)\n") {
		t.Errorf("output:\n%s", out.String())
	}
	if after := files(t, dir); !slices.Equal(before, after) {
		t.Errorf("DryRun changed the files: %v, then %v", before, after)
	}
}

func TestDryRunGivesTheErrorsOfInit(t *testing.T) {
	for name, file := range map[string]struct{ path, content string }{
		"settings not JSON":     {claude.SettingsPath, "{not json"},
		"settings wrong shape":  {claude.SettingsPath, `{"hooks": {"PostToolUse": {}}}`},
		"specs path is a file":  {"specs", "not a directory\n"},
		"configured specs file": {config.FileName, "specs_dir: plan.md\n"},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.content), 0o644); err != nil {
			t.Fatal(err)
		}
		if name == "configured specs file" {
			if err := os.WriteFile(filepath.Join(dir, "plan.md"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		before := files(t, dir)
		dryErr := DryRun(dir, &bytes.Buffer{})
		initErr := Init(dir, &bytes.Buffer{})
		if dryErr == nil || initErr == nil || dryErr.Error() != initErr.Error() {
			t.Errorf("%s: DryRun error %v, Init error %v", name, dryErr, initErr)
		}
		// Init fails before it writes anything.
		if after := files(t, dir); !slices.Equal(before, after) {
			t.Errorf("%s: the files changed: %v, then %v", name, before, after)
		}
	}
}

func TestInitKeepsTheRulesOfAProject(t *testing.T) {
	dir := t.TempDir()
	rules := filepath.Join(dir, ".specval", "policy")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rules, "naming.rego"), []byte("package specval.spec.naming\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Init(dir, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "exists   .specval/policy/naming.rego (rules)\n") {
		t.Errorf("output:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(rules, "project.rego")); !os.IsNotExist(err) {
		t.Errorf("Init made project.rego: %v", err)
	}
	// A Rego test file is not a rule, so Init adds the example rule file.
	dir = t.TempDir()
	rules = filepath.Join(dir, ".specval", "policy")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rules, "naming_test.rego"), []byte("package specval.spec.naming_test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(rules, "project.rego")); err != nil {
		t.Errorf("Init did not make project.rego: %v", err)
	}
}
