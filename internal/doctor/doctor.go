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

// Status is the result of a check.
type Status string

// The values of Status.
const (
	StatusOK   Status = "ok"   // The part works.
	StatusWarn Status = "warn" // The part can cause a problem.
	StatusFail Status = "fail" // The part does not work.
)

// Check is the result of one check.
type Check struct {
	Status Status `json:"status"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Env is what the checks read from the system.
type Env struct {
	// Dir is the directory to check from. The checks find the project
	// configuration in Dir or in a parent of Dir.
	Dir string
	// Home is the home directory of the user. The checks look there for the
	// user-level skill and settings. If it is "", they look only in the project.
	Home string
	// Executable is the path of the specval binary that runs the checks.
	Executable string
}

const repair = "Run `specval init` in the project root; it restores missing pieces without overwriting anything."

// Run checks the project that contains env.Dir. It returns the checks in
// this order: config, specs, data and policy paths, rules, registry, skill,
// hook, and path. If there is no configuration, it returns only the config check.
func Run(ctx context.Context, env Env) []Check {
	cfg, err := config.Find(env.Dir)
	if err != nil {
		return []Check{{StatusFail, "config", err.Error(), "Run `specval init` in the project root."}}
	}
	c := &checker{env: env, cfg: cfg}
	c.add(StatusOK, "config", filepath.Join(cfg.Root, config.FileName), "")
	c.checkSpecs()
	c.checkPaths()
	c.checkRegistry(ctx, c.checkRules(ctx))
	c.checkSkill()
	c.checkHook()
	c.checkPath(ctx)
	return c.checks
}

// checker holds the state of one Run.
type checker struct {
	env    Env
	cfg    *config.Config
	checks []Check
}

func (c *checker) add(status Status, name, detail, fix string) {
	c.checks = append(c.checks, Check{status, name, detail, fix})
}

// rel returns p relative to the project root, if possible.
func (c *checker) rel(p string) string {
	if r, err := filepath.Rel(c.cfg.Root, p); err == nil {
		return r
	}
	return p
}

// checkSpecs makes sure that the specs directory exists, and counts the specs.
func (c *checker) checkSpecs() {
	if info, err := os.Stat(c.cfg.Abs(c.cfg.SpecsDir)); err != nil || !info.IsDir() {
		c.add(StatusWarn, "specs", c.cfg.SpecsDir+" does not exist", repair)
	} else if specs, err := c.cfg.SpecFiles(); err != nil {
		c.add(StatusFail, "specs", err.Error(), "")
	} else {
		c.add(StatusOK, "specs", fmt.Sprintf("%s (%d spec(s))", c.cfg.SpecsDir, len(specs)), "")
	}
}

// checkPaths makes sure that each data and policy path in the configuration
// exists. It adds a check only for a path that does not exist.
func (c *checker) checkPaths() {
	for _, group := range []struct {
		name  string
		paths []string
	}{{"data", c.cfg.Data}, {"policy", c.cfg.Policy}} {
		for _, p := range group.paths {
			if _, err := os.Stat(c.cfg.Abs(p)); err != nil {
				c.add(StatusWarn, group.name, p+" (listed in "+config.FileName+") does not exist", repair)
			}
		}
	}
}

// checkRules loads the data and compiles the rules. It returns nil if they
// do not load.
func (c *checker) checkRules(ctx context.Context) *policy.Engine {
	eng, err := policy.New(ctx, c.cfg)
	if err != nil {
		c.add(StatusFail, "rules", strings.ReplaceAll(err.Error(), c.cfg.Root+string(filepath.Separator), ""), "Fix the file named above; `specval check` will fail until then.")
		return nil
	}
	docs := eng.RuleDocs()
	var project int
	for _, d := range docs {
		if !d.Baseline {
			project++
		}
	}
	c.add(StatusOK, "rules", fmt.Sprintf("%d spec rule(s) compile, %d from this project", len(docs), project), "")
	return eng
}

// checkRegistry makes sure that services are registered and that they pass
// the registry rules. If eng is nil, it cannot check the registry.
func (c *checker) checkRegistry(ctx context.Context, eng *policy.Engine) {
	if eng == nil {
		c.add(StatusWarn, "registry", "not checked until the rules load", "")
		return
	}
	services, err := eng.Services()
	switch {
	case err != nil:
		c.add(StatusFail, "registry", err.Error(), "")
		return
	case len(services) == 0:
		c.add(StatusWarn, "registry", "no services registered (data.project.services is empty)",
			"Add your services and their layers to .specval/data/services.yaml; until then every call fails SV101.")
		return
	}
	reg, err := eng.CheckRegistry(ctx)
	if err != nil {
		c.add(StatusFail, "registry", err.Error(), "")
		return
	}
	var problems []string
	for file, found := range reg {
		for _, f := range found {
			problems = append(problems, fmt.Sprintf("%s:%d %s", c.rel(file), f.Line, f.Msg))
		}
	}
	if len(problems) > 0 {
		c.add(StatusFail, "registry", strings.Join(problems, "; "), "Run `specval check` for the fix for each problem.")
	} else {
		c.add(StatusOK, "registry", fmt.Sprintf("%d service(s), %s", len(services), c.rel(eng.DataFile("services"))), "")
	}
}

// checkSkill makes sure that the project or the user has the specval skill,
// and that the skill still tells Claude to run `specval context`. Claude Code
// reads the skill from the project first, then from the home directory.
func (c *checker) checkSkill() {
	paths := []string{filepath.Join(c.cfg.Root, claude.SkillPath)}
	if c.env.Home != "" {
		paths = append(paths, filepath.Join(c.env.Home, claude.SkillPath))
	}
	skill := ""
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			skill = p
			break
		}
	}
	switch b, _ := os.ReadFile(skill); {
	case skill == "":
		c.add(StatusWarn, "skill", claude.SkillPath+" is missing; Claude won't know to load the rules while planning", repair)
	case !strings.Contains(string(b), "specval context"):
		c.add(StatusWarn, "skill", c.rel(skill)+" no longer tells Claude to run `specval context`", "Restore it from the template: delete it and run `specval init`.")
	default:
		c.add(StatusOK, "skill", c.rel(skill), "")
	}
}

// checkHook makes sure that a settings file runs `specval hook` after each
// file write, and that no settings file turns hooks off. Claude Code reads
// the project settings, the local project settings, and then the user
// settings.
func (c *checker) checkHook() {
	paths := []string{filepath.Join(c.cfg.Root, claude.SettingsPath), filepath.Join(c.cfg.Root, claude.LocalSettingsPath)}
	if c.env.Home != "" {
		paths = append(paths, filepath.Join(c.env.Home, claude.SettingsPath))
	}
	var hookFile, disabledIn string
	var hook claude.HookStatus
	for _, p := range paths {
		st, err := claude.InspectSettings(p)
		if err != nil {
			c.add(StatusFail, "hook", strings.ReplaceAll(err.Error(), c.cfg.Root+string(filepath.Separator), ""), "Fix the JSON; Claude Code ignores a settings file it can't parse.")
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
		c.add(StatusFail, "hook", "no PostToolUse hook runs `specval hook`; specs Claude writes won't be checked", repair)
		return
	case disabledIn != "":
		c.add(StatusFail, "hook", "hooks are turned off by disableAllHooks in "+c.rel(disabledIn), "Remove disableAllHooks from that file.")
		return
	}
	var missed []string
	for _, tool := range []string{"Write", "Edit", "MultiEdit"} {
		if !claude.MatcherCovers(hook.Matcher, tool) {
			missed = append(missed, tool)
		}
	}
	if len(missed) > 0 {
		c.add(StatusWarn, "hook", fmt.Sprintf("matcher %q in %s doesn't fire on %s", hook.Matcher, c.rel(hookFile), strings.Join(missed, ", ")),
			`Set the matcher to "Write|Edit|MultiEdit".`)
	} else {
		c.add(StatusOK, "hook", c.rel(hookFile), "")
	}
}

// checkPath makes sure that the hook can run specval. The hook runs the
// specval that is on PATH, which can be different from the one that runs
// doctor.
func (c *checker) checkPath(ctx context.Context) {
	onPath, err := exec.LookPath("specval")
	if err != nil {
		c.add(StatusFail, "path", "specval is not on PATH, so the hook command `specval hook` can't run",
			"Put the specval binary on PATH (the same PATH Claude Code starts with).")
	} else if out, err := exec.CommandContext(ctx, onPath, "help").CombinedOutput(); err != nil { //nolint:gosec // This check must run the specval binary that is on PATH.
		c.add(StatusFail, "path", fmt.Sprintf("%s doesn't run: %v %s", onPath, err, strings.TrimSpace(string(out))), "Reinstall specval.")
	} else if !sameFile(c.env.Executable, onPath) {
		c.add(StatusWarn, "path", fmt.Sprintf("the hook will run %s, but you ran %s", onPath, c.env.Executable), "Make sure they're the same version, or fix PATH.")
	} else {
		c.add(StatusOK, "path", onPath, "")
	}
}

// sameFile reports whether a and b are the same file.
func sameFile(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}
