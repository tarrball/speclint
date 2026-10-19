package doctor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarrball/speclint/internal/claude"
	"github.com/tarrball/speclint/internal/copilot"
	"github.com/tarrball/speclint/internal/scaffold"
)

// fixture is a project that passes each check.
type fixture struct {
	env Env
	t   *testing.T
}

// newFixture makes a project with scaffold.Init and a registry, a home
// directory, and a fake slint on PATH. The tests that use it cannot run in
// parallel, because it sets PATH.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "project")
	bin := filepath.Join(root, "bin")
	f := &fixture{t: t, env: Env{Dir: dir, Home: filepath.Join(root, "home"), Executable: filepath.Join(bin, "slint")}}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.Init(dir, &bytes.Buffer{}, scaffold.Options{Claude: true}); err != nil {
		t.Fatal(err)
	}
	f.write(".speclint/data/services.yaml", "services:\n  OrderManager: { layer: manager }\n")
	// doctor runs `slint help` and looks for `slint hook` in the usage. It runs
	// `slint version` to show the version.
	f.writeExecutable(f.env.Executable, fakeSv("v9.9.9"))
	t.Setenv("PATH", bin)
	return f
}

// fakeSv returns a script that answers `slint version` with version and each
// other command with the usage line that doctor looks for.
func fakeSv(version string) string {
	return "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'slint " + version + " (spec format 1)'; else echo '  slint hook'; fi\n"
}

// write writes a file at a path relative to the project directory.
func (f *fixture) write(path, content string) {
	f.t.Helper()
	p := filepath.Join(f.env.Dir, path)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) writeExecutable(path, content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil { // The fake binary must be executable.
		f.t.Fatal(err)
	}
}

func (f *fixture) remove(path string) {
	f.t.Helper()
	if err := os.RemoveAll(filepath.Join(f.env.Dir, path)); err != nil {
		f.t.Fatal(err)
	}
}

// want runs the checks and fails the test unless a check with the given
// name has the given status and a detail that contains detail.
func (f *fixture) want(name string, status Status, detail string) {
	f.t.Helper()
	checks := Run(context.Background(), f.env)
	for _, c := range checks {
		if c.Name == name && c.Status == status && strings.Contains(c.Detail, detail) {
			return
		}
	}
	f.t.Errorf("no %s check with status %s and detail %q in:\n%+v", name, status, detail, checks)
}

func TestHealthyProject(t *testing.T) {
	f := newFixture(t)
	checks := Run(context.Background(), f.env)
	var names []string
	for _, c := range checks {
		names = append(names, c.Name)
		if c.Status != StatusOK {
			t.Errorf("%s: %s %s", c.Name, c.Status, c.Detail)
		}
	}
	if got := strings.Join(names, " "); got != "config specs rules registry copilot skill hook path" {
		t.Errorf("checks = %s", got)
	}
}

func TestNoConfig(t *testing.T) {
	checks := Run(context.Background(), Env{Dir: t.TempDir()})
	if len(checks) != 1 || checks[0].Name != "config" || checks[0].Status != StatusFail {
		t.Errorf("checks = %+v, want only a failed config check", checks)
	}
}

func TestProjectChecks(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(f *fixture)
		check  string
		status Status
		detail string
	}{
		{"specs directory missing", func(f *fixture) { f.remove("specs") }, "specs", StatusWarn, "specs does not exist"},
		{"spec count", func(f *fixture) { f.write("specs/A-1.yaml", "") }, "specs", StatusOK, "specs (1 spec(s))"},
		{"policy directory missing", func(f *fixture) { f.remove(".speclint/policy") }, "policy", StatusWarn, ".speclint/policy (listed in .speclint.yaml) does not exist"},
		{"rule does not compile", func(f *fixture) {
			f.write(".speclint/policy/x.rego", "package speclint.spec.x\n\ndeny contains f if {\n")
		}, "rules", StatusFail, ".speclint/policy/x.rego:"},
		{"registry not checked", func(f *fixture) {
			f.write(".speclint/policy/x.rego", "package speclint.spec.x\n\ndeny contains f if {\n")
		}, "registry", StatusWarn, "cannot check the registry until the rules load"},
		{"project rule count", func(f *fixture) {
			f.write(".speclint/policy/x.rego", "package speclint.spec.x\n\nimport rego.v1\n\n# METADATA\n# title: X\n# custom:\n#   id: X001\ndeny contains \"x\" if false\n")
		}, "rules", StatusOK, "compile, 1 from this project"},
		{"project rule in a baseline package", func(f *fixture) {
			f.write(".speclint/policy/x.rego", "package speclint.spec.dpl\n\nimport rego.v1\n\n# METADATA\n# title: X\n# custom:\n#   id: X001\ndeny contains \"x\" if false\n")
		}, "rules", StatusOK, "compile, 1 from this project"},
		{"registry empty", func(f *fixture) { f.write(".speclint/data/services.yaml", "services: {}\n") }, "registry", StatusWarn, "no services are registered"},
		{"registry not a mapping", func(f *fixture) { f.write(".speclint/data/services.yaml", "services: [a]\n") }, "registry", StatusFail, "data.project.services must be a mapping"},
		{"registry rule warns", func(f *fixture) {
			f.write(".speclint/policy/r.rego", "package speclint.registry.r\n\nimport data.speclint.lib\nimport rego.v1\n\n"+
				"# METADATA\n# title: T\n# custom:\n#   id: PRJ900\n"+
				"warn contains lib.finding(rego.metadata.rule(), \"services.OrderManager\", \"OrderManager has no owner\", \"Add an owner.\") if true\n")
		}, "registry", StatusWarn, "OrderManager has no owner"},
		{"registry rule fails", func(f *fixture) {
			f.write(".speclint/data/services.yaml", "services:\n  PricingThing: { layer: engine }\n")
		}, "registry", StatusFail, ".speclint/data/services.yaml:2 PricingThing is in the engine layer"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			c.change(f)
			f.want(c.check, c.status, c.detail)
		})
	}
}

func TestClaudeChecks(t *testing.T) {
	narrow := `{"hooks": {"PostToolUse": [{"matcher": "Write", "hooks": [{"command": "slint hook"}]}]}}`
	for _, c := range []struct {
		name   string
		change func(f *fixture)
		check  string
		status Status
		detail string
	}{
		{"skill missing", func(f *fixture) { f.remove(claude.SkillPath) }, "skill", StatusWarn, "is missing"},
		{"skill changed", func(f *fixture) { f.write(claude.SkillPath, "Write the code.\n") }, "skill", StatusWarn, "no longer tells Claude to run `slint context`"},
		{"skill cannot be read", func(f *fixture) {
			f.remove(claude.SkillPath)
			f.write(filepath.Join(claude.SkillPath, "x"), "") // The skill path is now a directory.
		}, "skill", StatusFail, "SKILL.md"},
		{"skill in home", func(f *fixture) {
			f.remove(claude.SkillPath)
			f.write(filepath.Join("..", "home", claude.SkillPath), claude.Skill("specs"))
		}, "skill", StatusOK, "home"},
		{"copilot instructions missing", func(f *fixture) { f.remove(copilot.InstructionsPath) }, "copilot", StatusWarn, "is missing, so Copilot does not compare a pull request with its spec"},
		{"copilot instructions cannot be read", func(f *fixture) {
			f.remove(copilot.InstructionsPath)
			f.write(filepath.Join(copilot.InstructionsPath, "x"), "") // The instructions path is now a directory.
		}, "copilot", StatusFail, "speclint.instructions.md"},
		{"hook missing", func(f *fixture) { f.remove(claude.SettingsPath) }, "hook", StatusFail, "no PostToolUse hook runs `slint hook`"},
		{"hook in home", func(f *fixture) {
			f.remove(claude.SettingsPath)
			f.write(filepath.Join("..", "home", claude.SettingsPath), narrow)
		}, "hook", StatusWarn, "home/.claude/settings.json"},
		{"hook in local settings", func(f *fixture) {
			f.remove(claude.SettingsPath)
			f.write(claude.LocalSettingsPath, narrow)
		}, "hook", StatusWarn, `matcher "Write" in .claude/settings.local.json does not match Edit, MultiEdit`},
		{"hooks disabled", func(f *fixture) { f.write(claude.LocalSettingsPath, `{"disableAllHooks": true}`) }, "hook", StatusFail, "disableAllHooks in .claude/settings.local.json turns off all hooks"},
		{"settings not JSON", func(f *fixture) { f.write(claude.LocalSettingsPath, "{not json") }, "hook", StatusFail, ".claude/settings.local.json is not valid JSON"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			c.change(f)
			f.want(c.check, c.status, c.detail)
		})
	}
}

func TestUnknownHomeDirectory(t *testing.T) {
	// With no home directory, doctor must not look in the working directory
	// of the process for the user-level files.
	f := newFixture(t)
	f.remove(claude.SettingsPath)
	f.remove(claude.SkillPath)
	f.env.Home = ""
	cwd := t.TempDir()
	for path, content := range map[string]string{
		claude.SettingsPath: `{"hooks": {"PostToolUse": [{"matcher": "Write|Edit|MultiEdit", "hooks": [{"command": "slint hook"}]}]}}`,
		claude.SkillPath:    claude.Skill("specs"),
	} {
		p := filepath.Join(cwd, path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(cwd)
	// The files in the working directory do not count, so Claude Code is not
	// set up.
	f.want("claude", StatusOK, "not set up")
}

func TestClaudeNotSetUp(t *testing.T) {
	f := newFixture(t)
	f.remove(claude.SettingsPath)
	f.remove(claude.SkillPath)
	checks := Run(context.Background(), f.env)
	var names []string
	for _, c := range checks {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, " "); got != "config specs rules registry copilot claude" {
		t.Errorf("checks = %s", got)
	}
	f.want("claude", StatusOK, "not set up; to use speclint with Claude Code, run `slint init --claude`")
}

func TestSettingsThatAreNotValidDoNotSetUpClaude(t *testing.T) {
	f := newFixture(t)
	f.remove(claude.SkillPath)
	f.write(claude.SettingsPath, "{not json")
	f.want("claude", StatusOK, "not set up")
}

func TestClaudeRepairTellsToUseTheFlag(t *testing.T) {
	f := newFixture(t)
	f.remove(claude.SettingsPath)
	for _, c := range Run(context.Background(), f.env) {
		if c.Name == "hook" && !strings.Contains(c.Fix, "slint init --claude") {
			t.Errorf("hook fix = %q, want slint init --claude", c.Fix)
		}
	}
	f.want("hook", StatusFail, "no PostToolUse hook runs `slint hook`")
}

func TestRegistryCountsTheLayerBreaks(t *testing.T) {
	f := newFixture(t)
	f.write(".speclint/data/services.yaml", `services:
  OrderApi: { layer: client, calls: [OrderAccessor] }
  OrderManager: { layer: manager, calls: [OrderAccessor] }
  OrderAccessor: { layer: accessor, calls: [BillingAccessor] }
  BillingAccessor: { layer: accessor, calls: [OrderAccessor] }
`)
	f.want("registry", StatusOK, "4 service(s), .speclint/data/services.yaml; 3 registered call(s) break the layer rules (accessor -> accessor 2, client -> accessor 1)")
}

func TestRegistryCountsTheLayerBreaksWithARegistryProblem(t *testing.T) {
	f := newFixture(t)
	f.write(".speclint/data/services.yaml", `services:
  OrderApi: { layer: client, calls: [OrderAccessor, Nobody] }
  OrderAccessor: { layer: accessor }
`)
	f.want("registry", StatusFail, `"Nobody" in calls of OrderApi is not a registered service; 1 registered call(s) break the layer rules (client -> accessor 1)`)
}

func TestPathChecks(t *testing.T) {
	t.Run("not on PATH", func(t *testing.T) {
		f := newFixture(t)
		t.Setenv("PATH", t.TempDir())
		f.want("path", StatusFail, "slint is not on PATH")
	})
	t.Run("does not run", func(t *testing.T) {
		f := newFixture(t)
		f.writeExecutable(f.env.Executable, "#!/bin/sh\necho broken\nexit 3\n")
		f.want("path", StatusFail, "does not run: exit status 3 broken")
	})
	t.Run("another program named slint", func(t *testing.T) {
		f := newFixture(t)
		f.writeExecutable(f.env.Executable, "#!/bin/sh\necho 'usage: slint [-v] [-w sec] command service ...'\n")
		f.want("path", StatusFail, "is not speclint; another program has the name slint")
	})
	t.Run("the same binary", func(t *testing.T) {
		f := newFixture(t)
		f.want("path", StatusOK, f.env.Executable+" (v9.9.9)")
	})
	t.Run("a different binary", func(t *testing.T) {
		f := newFixture(t)
		ran := f.env.Executable
		f.env.Executable = filepath.Join(t.TempDir(), "slint")
		f.writeExecutable(f.env.Executable, fakeSv("v1.0.0"))
		f.want("path", StatusWarn, "the hook will run "+ran+" (v9.9.9), but you ran "+f.env.Executable+" (v1.0.0)")
	})
	t.Run("no version command", func(t *testing.T) {
		f := newFixture(t)
		f.writeExecutable(f.env.Executable, "#!/bin/sh\necho '  slint hook'\n")
		f.want("path", StatusOK, f.env.Executable+" (unknown version)")
	})
}

// Each check has one result. If a settings file is not valid, the hook check
// fails, and doctor does not also report the hook as correct.
func TestOneHookCheckWhenASettingsFileIsNotValid(t *testing.T) {
	f := newFixture(t)
	f.write(claude.LocalSettingsPath, "{not json")
	var hooks []Check
	for _, c := range Run(context.Background(), f.env) {
		if c.Name == "hook" {
			hooks = append(hooks, c)
		}
	}
	if len(hooks) != 1 || hooks[0].Status != StatusFail {
		t.Errorf("hook checks = %+v, want one failure", hooks)
	}
}

func TestRegistryFixNamesTheConfiguredDataFile(t *testing.T) {
	f := newFixture(t)
	f.remove(".speclint/data")
	f.write(".speclint.yaml", "data: [registry.yaml]\n")
	f.write("registry.yaml", "services: {}\n")
	for _, c := range Run(context.Background(), f.env) {
		if c.Name == "registry" && !strings.Contains(c.Fix, "registry.yaml") {
			t.Errorf("registry fix = %q, want it to name registry.yaml", c.Fix)
		}
	}
}
