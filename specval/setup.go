package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	skillPath    = ".claude/skills/spec/SKILL.md"
	settingsPath = ".claude/settings.json"
)

// cmdInit creates whatever parts of the setup are missing. It never overwrites
// a file, so it is also the repair step `specval doctor` points to.
func cmdInit() error {
	specsDir := "specs"
	if cfg, err := findConfig("."); err == nil && sameDir(cfg.Root, ".") {
		specsDir = cfg.SpecsDir
	}
	files := []struct{ path, tmpl string }{
		{configName, "templates/config.yaml"},
		{".specval/data/services.yaml", "templates/services.yaml"},
		{".specval/policy/project.rego", "templates/project.rego"},
		{skillPath, "templates/SKILL.md"},
	}
	for _, f := range files {
		if err := writeIfMissing(f.path, f.tmpl); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return err
	}
	added, err := addHook(settingsPath)
	if err != nil {
		return err
	}
	if added {
		fmt.Println("hooked  ", settingsPath)
	} else {
		fmt.Println("exists  ", settingsPath, "(specval hook)")
	}
	return nil
}

func writeIfMissing(path, tmpl string) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Println("exists  ", path)
		return nil
	}
	b, _ := embedded.ReadFile(tmpl)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	fmt.Println("created ", path)
	return nil
}

func sameDir(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return a == b
}

// --- Claude Code settings ---

type settingsFile struct {
	DisableAllHooks bool `json:"disableAllHooks"`
	Hooks           map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

type hookStatus struct {
	exists, found, disabled bool
	matcher                 string
}

// inspectSettings reports whether a Claude settings file runs `specval hook` on PostToolUse.
func inspectSettings(path string) (hookStatus, error) {
	var st hookStatus
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	} else if err != nil {
		return st, err
	}
	st.exists = true
	var s settingsFile
	if err := json.Unmarshal(b, &s); err != nil {
		return st, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	st.disabled = s.DisableAllHooks
	for _, entry := range s.Hooks["PostToolUse"] {
		for _, h := range entry.Hooks {
			if strings.Contains(h.Command, "specval hook") {
				st.found, st.matcher = true, entry.Matcher
				return st, nil
			}
		}
	}
	return st, nil
}

// addHook merges the specval PostToolUse hook into a settings file, creating it if needed.
func addHook(path string) (bool, error) {
	st, err := inspectSettings(path)
	if err != nil || st.found {
		return false, err
	}
	s := map[string]any{}
	if st.exists {
		b, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		if err := json.Unmarshal(b, &s); err != nil {
			return false, err
		}
	}
	var tmpl struct {
		Hooks struct {
			PostToolUse []any `json:"PostToolUse"`
		} `json:"hooks"`
	}
	b, _ := embedded.ReadFile("templates/settings.json")
	if err := json.Unmarshal(b, &tmpl); err != nil {
		return false, err
	}
	hooks, _ := s["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		s["hooks"] = hooks
	}
	post, _ := hooks["PostToolUse"].([]any)
	hooks["PostToolUse"] = append(post, tmpl.Hooks.PostToolUse...)
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o644)
}

// matcherCovers reports whether a hook matcher fires for a tool name.
// An empty matcher or "*" matches every tool.
func matcherCovers(matcher, tool string) bool {
	if matcher == "" || matcher == "*" {
		return true
	}
	re, err := regexp.Compile("^(?:" + matcher + ")$")
	return err == nil && re.MatchString(tool)
}

// --- doctor ---

type Check struct {
	Status string `json:"status"` // ok, warn, fail
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

const repair = "Run `specval init` in the project root; it restores missing pieces without overwriting anything."

func cmdDoctor(ctx context.Context, args []string) (int, error) {
	fl := flag.NewFlagSet("doctor", flag.ContinueOnError)
	format := fl.String("format", "text", "output format: text or json")
	if _, err := parseArgs(fl, args); err != nil {
		return 2, err
	}
	cwd, _ := os.Getwd()
	checks := doctor(ctx, cwd)
	failed := false
	for _, c := range checks {
		failed = failed || c.Status == "fail"
	}
	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"checks": checks, "ok": !failed})
	case "text":
		printChecks(os.Stdout, checks)
	default:
		return 2, fmt.Errorf("unknown format %q", *format)
	}
	if failed {
		return 1, nil
	}
	return 0, nil
}

func printChecks(w io.Writer, checks []Check) {
	marks := map[string]string{"ok": "✓", "warn": "!", "fail": "✗"}
	var warns, fails int
	for _, c := range checks {
		fmt.Fprintf(w, "%s %-9s %s\n", marks[c.Status], c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(w, "    fix: %s\n", c.Fix)
		}
		switch c.Status {
		case "warn":
			warns++
		case "fail":
			fails++
		}
	}
	if fails+warns == 0 {
		fmt.Fprintln(w, "specval setup looks good.")
	} else {
		fmt.Fprintf(w, "%d problem(s), %d warning(s).\n", fails, warns)
	}
}

func doctor(ctx context.Context, cwd string) []Check {
	var checks []Check
	add := func(status, name, detail, fix string) {
		checks = append(checks, Check{status, name, detail, fix})
	}

	cfg, err := findConfig(cwd)
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
	add("ok", "config", filepath.Join(cfg.Root, configName), "")

	// specs dir
	var specs int
	specsDir := cfg.abs(cfg.SpecsDir)
	if info, err := os.Stat(specsDir); err != nil || !info.IsDir() {
		add("warn", "specs", cfg.SpecsDir+" does not exist", repair)
	} else {
		walkFiles(specsDir, []string{".yaml", ".yml"}, func(string) error { specs++; return nil })
		add("ok", "specs", fmt.Sprintf("%s (%d spec(s))", cfg.SpecsDir, specs), "")
	}

	// configured data/policy paths
	for _, group := range []struct {
		name  string
		paths []string
	}{{"data", cfg.Data}, {"policy", cfg.Policy}} {
		for _, p := range group.paths {
			if _, err := os.Stat(cfg.abs(p)); err != nil {
				add("warn", group.name, p+" (listed in "+configName+") does not exist", repair)
			}
		}
	}

	// rules + data load and compile
	eng, err := newEngine(ctx, cfg)
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

		projectData, _ := eng.data["project"].(map[string]any)
		services, _ := projectData["services"].(map[string]any)
		if len(services) == 0 {
			add("warn", "registry", "no services registered (data.project.services is empty)",
				"Add your services and their layers to .specval/data/services.yaml; until then every call fails SV101.")
		} else if reg, err := eng.CheckRegistry(ctx); err != nil {
			add("fail", "registry", err.Error(), "")
		} else {
			var problems []string
			for file, fs := range reg {
				for _, f := range fs {
					problems = append(problems, fmt.Sprintf("%s:%d %s", rel(file), f.Line, f.Msg))
				}
			}
			if len(problems) > 0 {
				add("fail", "registry", strings.Join(problems, "; "), "Run `specval check` for the fix for each problem.")
			} else {
				add("ok", "registry", fmt.Sprintf("%d service(s), %s", len(services), rel(eng.dataFile["services"])), "")
			}
		}
	}

	// Claude skill (project or user level)
	home, _ := os.UserHomeDir()
	skill := ""
	for _, p := range []string{filepath.Join(cfg.Root, skillPath), filepath.Join(home, skillPath)} {
		if _, err := os.Stat(p); err == nil {
			skill = p
			break
		}
	}
	switch b, _ := os.ReadFile(skill); {
	case skill == "":
		add("warn", "skill", skillPath+" is missing; Claude won't know to load the rules while planning", repair)
	case !strings.Contains(string(b), "specval context"):
		add("warn", "skill", rel(skill)+" no longer tells Claude to run `specval context`", "Restore it from the template: delete it and run `specval init`.")
	default:
		add("ok", "skill", rel(skill), "")
	}

	// Claude hook: project, local, then user settings
	var hookFile string
	var hook hookStatus
	disabledIn := ""
	for _, p := range []string{
		filepath.Join(cfg.Root, settingsPath),
		filepath.Join(cfg.Root, ".claude/settings.local.json"),
		filepath.Join(home, settingsPath),
	} {
		st, err := inspectSettings(p)
		if err != nil {
			add("fail", "hook", strings.ReplaceAll(err.Error(), cfg.Root+string(filepath.Separator), ""), "Fix the JSON; Claude Code ignores a settings file it can't parse.")
			continue
		}
		if st.disabled && disabledIn == "" {
			disabledIn = p
		}
		if st.found && hookFile == "" {
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
			if !matcherCovers(hook.matcher, tool) {
				missed = append(missed, tool)
			}
		}
		if len(missed) > 0 {
			add("warn", "hook", fmt.Sprintf("matcher %q in %s doesn't fire on %s", hook.matcher, rel(hookFile), strings.Join(missed, ", ")),
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
	} else if out, err := exec.CommandContext(ctx, onPath, "help").CombinedOutput(); err != nil {
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
