// Package doctor checks that a specval project and its Claude Code setup work.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tarrball/specval/internal/claude"
	"github.com/tarrball/specval/internal/config"
	"github.com/tarrball/specval/internal/policy"
)

// Check is the result of one check.
type Check struct {
	Status string `json:"status"` // ok, warn, fail
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

const repair = "Run `specval init` in the project root; it restores missing pieces without overwriting anything."

// Run checks the project that contains cwd.
func Run(ctx context.Context, cwd string) []Check {
	var checks []Check
	add := func(status, name, detail, fix string) {
		checks = append(checks, Check{status, name, detail, fix})
	}

	cfg, err := config.Find(cwd)
	if err != nil {
		add("fail", "config", err.Error(), "Run `specval init` in the project root.")
		return checks
	}
	rel := func(p string) string {
		if r, err := filepath.Rel(cfg.Root, p); err == nil {
			return r
		}
		return p
	}
	add("ok", "config", filepath.Join(cfg.Root, config.FileName), "")

	// specs dir
	specsDir := cfg.Abs(cfg.SpecsDir)
	if info, err := os.Stat(specsDir); err != nil || !info.IsDir() {
		add("warn", "specs", cfg.SpecsDir+" does not exist", repair)
	} else if specs, err := cfg.SpecFiles(); err != nil {
		add("fail", "specs", err.Error(), "")
	} else {
		add("ok", "specs", fmt.Sprintf("%s (%d spec(s))", cfg.SpecsDir, len(specs)), "")
	}

	// configured data/policy paths
	for _, group := range []struct {
		name  string
		paths []string
	}{{"data", cfg.Data}, {"policy", cfg.Policy}} {
		for _, p := range group.paths {
			if _, err := os.Stat(cfg.Abs(p)); err != nil {
				add("warn", group.name, p+" (listed in "+config.FileName+") does not exist", repair)
			}
		}
	}

	// rules + data load and compile
	eng, err := policy.New(ctx, cfg)
	if err != nil {
		add("fail", "rules", strings.ReplaceAll(err.Error(), cfg.Root+string(filepath.Separator), ""), "Fix the file named above; `specval check` will fail until then.")
		add("warn", "registry", "not checked until the rules load", "")
	} else {
		var project int
		for _, d := range eng.RuleDocs() {
			if d.Package != "data.specval.spec.schema" && d.Package != "data.specval.spec.dpl" {
				project++
			}
		}
		add("ok", "rules", fmt.Sprintf("%d spec rule(s) compile, %d from this project", len(eng.RuleDocs()), project), "")

		services, err := eng.Services()
		switch {
		case err != nil:
			add("fail", "registry", err.Error(), "")
		case len(services) == 0:
			add("warn", "registry", "no services registered (data.project.services is empty)",
				"Add your services and their layers to .specval/data/services.yaml; until then every call fails SV101.")
		default:
			reg, err := eng.CheckRegistry(ctx)
			if err != nil {
				add("fail", "registry", err.Error(), "")
				break
			}
			var problems []string
			for file, found := range reg {
				for _, f := range found {
					problems = append(problems, fmt.Sprintf("%s:%d %s", rel(file), f.Line, f.Msg))
				}
			}
			if len(problems) > 0 {
				add("fail", "registry", strings.Join(problems, "; "), "Run `specval check` for the fix for each problem.")
			} else {
				add("ok", "registry", fmt.Sprintf("%d service(s), %s", len(services), rel(eng.DataFile("services"))), "")
			}
		}
	}

	// Claude skill (project or user level)
	home, _ := os.UserHomeDir() // Without a home directory, only the project files are checked.
	skill := ""
	for _, p := range []string{filepath.Join(cfg.Root, claude.SkillPath), filepath.Join(home, claude.SkillPath)} {
		if _, err := os.Stat(p); err == nil {
			skill = p
			break
		}
	}
	switch b, _ := os.ReadFile(skill); {
	case skill == "":
		add("warn", "skill", claude.SkillPath+" is missing; Claude won't know to load the rules while planning", repair)
	case !strings.Contains(string(b), "specval context"):
		add("warn", "skill", rel(skill)+" no longer tells Claude to run `specval context`", "Restore it from the template: delete it and run `specval init`.")
	default:
		add("ok", "skill", rel(skill), "")
	}

	// Claude hook: project, local, then user settings
	var hookFile string
	var hook claude.HookStatus
	disabledIn := ""
	for _, p := range []string{
		filepath.Join(cfg.Root, claude.SettingsPath),
		filepath.Join(cfg.Root, claude.LocalSettingsPath),
		filepath.Join(home, claude.SettingsPath),
	} {
		st, err := claude.InspectSettings(p)
		if err != nil {
			add("fail", "hook", strings.ReplaceAll(err.Error(), cfg.Root+string(filepath.Separator), ""), "Fix the JSON; Claude Code ignores a settings file it can't parse.")
			continue
		}
		if st.Disabled && disabledIn == "" {
			disabledIn = p
		}
		if st.Found && hookFile == "" {
			hookFile, hook = p, st
		}
	}
	switch {
	case hookFile == "":
		add("fail", "hook", "no PostToolUse hook runs `specval hook`; specs Claude writes won't be checked", repair)
	case disabledIn != "":
		add("fail", "hook", "hooks are turned off by disableAllHooks in "+rel(disabledIn), "Remove disableAllHooks from that file.")
	default:
		var missed []string
		for _, tool := range []string{"Write", "Edit", "MultiEdit"} {
			if !claude.MatcherCovers(hook.Matcher, tool) {
				missed = append(missed, tool)
			}
		}
		if len(missed) > 0 {
			add("warn", "hook", fmt.Sprintf("matcher %q in %s doesn't fire on %s", hook.Matcher, rel(hookFile), strings.Join(missed, ", ")),
				`Set the matcher to "Write|Edit|MultiEdit".`)
		} else {
			add("ok", "hook", rel(hookFile), "")
		}
	}

	// The hook runs whatever `specval` is on PATH.
	onPath, err := exec.LookPath("specval")
	if err != nil {
		add("fail", "path", "specval is not on PATH, so the hook command `specval hook` can't run",
			"Put the specval binary on PATH (the same PATH Claude Code starts with).")
	} else if out, err := exec.CommandContext(ctx, onPath, "help").CombinedOutput(); err != nil { //nolint:gosec // This check must run the specval binary that is on PATH.
		add("fail", "path", fmt.Sprintf("%s doesn't run: %v %s", onPath, err, strings.TrimSpace(string(out))), "Reinstall specval.")
	} else if self, _ := os.Executable(); !sameFile(self, onPath) {
		add("warn", "path", fmt.Sprintf("the hook will run %s, but you ran %s", onPath, self), "Make sure they're the same version, or fix PATH.")
	} else {
		add("ok", "path", onPath, "")
	}
	return checks
}

func sameFile(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}
