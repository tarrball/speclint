// Package doctor checks that a speclint project and its Claude Code setup work.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tarrball/speclint/internal/claude"
	"github.com/tarrball/speclint/internal/config"
	"github.com/tarrball/speclint/internal/copilot"
	"github.com/tarrball/speclint/internal/policy"
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
	// Executable is the path of the speclint binary that runs the checks.
	Executable string
}

const repair = "Run `slint init` in the project root. It adds the missing parts and does not change the other files."

// Run checks the project that contains env.Dir. It returns the checks in
// this order: config, specs, data and policy paths, rules, registry, skill,
// hook, and path. If there is no configuration, it returns only the config check.
func Run(ctx context.Context, env Env) []Check {
	cfg, err := config.Find(env.Dir)
	if err != nil {
		return []Check{{StatusFail, "config", err.Error(), "Run `slint init` in the project root."}}
	}
	c := &checker{env: env, cfg: cfg}
	c.add(StatusOK, "config", filepath.Join(cfg.Root, config.FileName), "")
	c.checkSpecs()
	c.checkPaths()
	c.checkRegistry(ctx, c.checkRules(ctx))
	c.checkSkill()
	c.checkCopilot()
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

// relText removes the project root from text, so that the paths in an error
// message are relative to the project, as in the other checks.
func (c *checker) relText(text string) string {
	return strings.ReplaceAll(text, c.cfg.Root+string(filepath.Separator), "")
}

// checkSpecs makes sure that the specs directory exists, and counts the specs.
func (c *checker) checkSpecs() {
	if info, err := os.Stat(c.cfg.Abs(c.cfg.SpecsDir)); err != nil || !info.IsDir() {
		c.add(StatusWarn, "specs", c.cfg.SpecsDir+" does not exist", repair)
	} else if specs, err := c.cfg.SpecFiles(); err != nil {
		c.add(StatusFail, "specs", err.Error(), "Make sure that speclint can read each file in "+c.cfg.SpecsDir+".")
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

// loadEngine reads the data files and the rule files of cfg, and makes an
// engine.
func loadEngine(ctx context.Context, cfg *config.Config) (*policy.Engine, error) {
	src, err := cfg.ReadSources()
	if err != nil {
		return nil, err
	}
	return policy.New(ctx, src.Data, src.Policies)
}

// checkRules loads the data and compiles the rules. It returns nil if they
// do not load.
func (c *checker) checkRules(ctx context.Context) *policy.Engine {
	eng, err := loadEngine(ctx, c.cfg)
	if err != nil {
		c.add(StatusFail, "rules", c.relText(err.Error()), "Fix the file in the error. Until you fix it, `slint check` fails.")
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
		c.add(StatusWarn, "registry", "speclint cannot check the registry until the rules load", "")
		return
	}
	services, err := eng.Services()
	switch {
	case err != nil:
		c.add(StatusFail, "registry", err.Error(),
			"Write services as a mapping from each service name to its entry, for example OrderManager: { layer: manager }.")
		return
	case len(services) == 0:
		c.add(StatusWarn, "registry", "no services are registered (data.project.services is empty)",
			"Add each service and its layer to "+c.registryFile(eng)+". Until you do, each call fails SV101.")
		return
	}
	reg, err := eng.CheckRegistry(ctx)
	if err != nil {
		c.add(StatusFail, "registry", c.relText(err.Error()), "Fix the registry rule in the error.")
		return
	}
	var problems []string
	status := StatusWarn
	for _, file := range slices.Sorted(maps.Keys(reg)) {
		for _, f := range reg[file] {
			where := c.rel(file)
			if f.Line > 0 {
				where = fmt.Sprintf("%s:%d", where, f.Line)
			}
			problems = append(problems, where+" "+f.Msg)
			if f.Severity == policy.SeverityError {
				status = StatusFail
			}
		}
	}
	breaks, err := eng.LayerBreaks()
	if err != nil {
		c.add(StatusFail, "registry", c.relText(err.Error()), "Run `slint check` to see the problem in the registry.")
		return
	}
	// A registry warning does not fail check, so it does not fail doctor.
	if len(problems) > 0 {
		c.add(status, "registry", strings.Join(problems, "; ")+layerBreaks(breaks), "Run `slint check` to see the fix for each problem.")
		return
	}
	c.add(StatusOK, "registry", fmt.Sprintf("%d service(s), %s%s", len(services), c.rel(eng.DataFile("services")), layerBreaks(breaks)), "")
}

// layerBreaks tells how many registered calls break the layer rules, for
// example "; 3 registered call(s) break the layer rules (accessor -> accessor 2,
// client -> accessor 1)". It returns "" if no call breaks them. Such calls are
// normal in an old project, and SV202 reports them when a spec changes the
// service, so they do not change the status of the check.
func layerBreaks(breaks []policy.LayerBreak) string {
	if len(breaks) == 0 {
		return ""
	}
	total := 0
	pairs := make([]string, len(breaks))
	for i, b := range breaks {
		total += b.Count
		pairs[i] = fmt.Sprintf("%s -> %s %d", b.From, b.To, b.Count)
	}
	return fmt.Sprintf("; %d registered call(s) break the layer rules (%s)", total, strings.Join(pairs, ", "))
}

// registryFile returns the data file that holds the registry: the file that
// defines services, or else the first data path of the configuration. If the
// configuration has no data path, it describes the file.
func (c *checker) registryFile(eng *policy.Engine) string {
	if f := eng.DataFile("services"); f != "" {
		return c.rel(f)
	}
	if len(c.cfg.Data) > 0 {
		return c.cfg.Data[0]
	}
	return "a data file that " + config.FileName + " lists"
}

// checkSkill makes sure that the project or the user has the speclint skill,
// and that the skill still tells Claude to run `slint context`. Claude Code
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
	if skill == "" {
		c.add(StatusWarn, "skill", claude.SkillPath+" is missing, so Claude does not know that it must read the rules before it plans", repair)
		return
	}
	b, err := os.ReadFile(skill)
	switch {
	case err != nil:
		c.add(StatusFail, "skill", c.relText(err.Error()), "Make sure that speclint can read the skill.")
	case !strings.Contains(string(b), "slint context"):
		c.add(StatusWarn, "skill", c.rel(skill)+" no longer tells Claude to run `slint context`", "Delete it, then run `slint init` to write it again from the template.")
	default:
		c.add(StatusOK, "skill", c.rel(skill), "")
	}
}

// checkCopilot makes sure that the project has the Copilot review
// instructions. A project that does not use Copilot can ignore the warning.
func (c *checker) checkCopilot() {
	// Read the file, so that a directory or a file that speclint cannot read
	// is a failure.
	switch _, err := os.ReadFile(filepath.Join(c.cfg.Root, copilot.InstructionsPath)); {
	case errors.Is(err, fs.ErrNotExist):
		c.add(StatusWarn, "copilot", copilot.InstructionsPath+" is missing, so Copilot does not compare a pull request with its spec", repair)
	case err != nil:
		c.add(StatusFail, "copilot", c.relText(err.Error()), "Make sure that speclint can read the instructions.")
	default:
		c.add(StatusOK, "copilot", copilot.InstructionsPath, "")
	}
}

// checkHook makes sure that a settings file runs `slint hook` after each
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
			c.add(StatusFail, "hook", c.relText(err.Error()), "Correct the file. Claude Code ignores a settings file that it cannot read.")
			return
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
		c.add(StatusFail, "hook", "no PostToolUse hook runs `slint hook`, so speclint does not check the specs that Claude writes", repair)
		return
	case disabledIn != "":
		c.add(StatusFail, "hook", "disableAllHooks in "+c.rel(disabledIn)+" turns off all hooks", "Remove disableAllHooks from that file.")
		return
	}
	var missed []string
	for _, tool := range []string{"Write", "Edit", "MultiEdit"} {
		if !claude.MatcherCovers(hook.Matcher, tool) {
			missed = append(missed, tool)
		}
	}
	if len(missed) > 0 {
		c.add(StatusWarn, "hook", fmt.Sprintf("matcher %q in %s does not match %s", hook.Matcher, c.rel(hookFile), strings.Join(missed, ", ")),
			`Set the matcher to "Write|Edit|MultiEdit".`)
	} else {
		c.add(StatusOK, "hook", c.rel(hookFile), "")
	}
}

// checkPath makes sure that the hook can run speclint. The hook runs the
// slint that is on PATH, which can be different from the one that runs doctor.
// Another program can also have the name slint.
func (c *checker) checkPath(ctx context.Context) {
	onPath, err := exec.LookPath("slint")
	if err != nil {
		c.add(StatusFail, "path", "slint is not on PATH, so Claude Code cannot run `slint hook`",
			"Put the slint binary in a directory on the PATH that Claude Code uses.")
		return
	}
	out, err := exec.CommandContext(ctx, onPath, "help").CombinedOutput() //nolint:gosec // This check must run the slint binary that is on PATH.
	switch {
	case err != nil:
		c.add(StatusFail, "path", fmt.Sprintf("%s does not run: %v %s", onPath, err, strings.TrimSpace(string(out))), "Install speclint again.")
	case !strings.Contains(string(out), "slint hook"):
		c.add(StatusFail, "path", onPath+" is not speclint; another program has the name slint",
			"Put the directory of the speclint binary before the directory of the other slint on PATH.")
	case !sameFile(c.env.Executable, onPath):
		c.add(StatusWarn, "path", fmt.Sprintf("the hook will run %s (%s), but you ran %s (%s)",
			onPath, versionOf(ctx, onPath), c.env.Executable, versionOf(ctx, c.env.Executable)),
			"Make sure that they are the same version, or change PATH.")
	default:
		c.add(StatusOK, "path", fmt.Sprintf("%s (%s)", onPath, versionOf(ctx, onPath)), "")
	}
}

// versionOf runs `slint version` and returns the version, for example v0.1.0.
// A slint from before the version command gives no version. Then versionOf
// returns "unknown version".
func versionOf(ctx context.Context, slint string) string {
	out, err := exec.CommandContext(ctx, slint, "version").Output()
	m := versionLine.FindStringSubmatch(string(out))
	if err != nil || m == nil {
		return "unknown version"
	}
	return m[1]
}

// versionLine is the output of `slint version`, for example
// "slint v0.1.0 (spec format 1)".
var versionLine = regexp.MustCompile(`^slint (\S+) \(spec format \d+\)\n$`)

// sameFile reports whether a and b are the same file.
func sameFile(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}
