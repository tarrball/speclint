// Command specval validates feature/bugfix specs against architecture rules
// before any code is written.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const usage = `specval validates specs against architecture rules before code is written.

Usage:
  specval check [--format text|json] [--strict] [files...]
        Validate specs (default: every spec in the specs dir). Exit 1 on errors.
  specval context
        Print the spec format, layer rules, registry, and rules (for an agent's context).
  specval new <id> [--kind feature|bugfix|chore] [--title "..."]
        Scaffold <specs dir>/<id>.yaml.
  specval schema
        Print the spec JSON Schema.
  specval init
        Set up specval and the Claude Code skill + hook in the current directory.
        Safe to re-run: it only adds what's missing.
  specval doctor [--format text|json]
        Check that the setup is intact (config, rules, registry, Claude skill, hook, PATH).
  specval hook
        Not run by hand: Claude Code runs it after each file write (set up by init).
        If the file is a spec, checks it and sends any errors back to Claude to fix.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx := context.Background()
	args := os.Args[2:]
	var code int
	var err error
	switch os.Args[1] {
	case "check":
		code, err = cmdCheck(ctx, args)
	case "context":
		err = cmdContext(ctx, os.Stdout)
	case "new":
		err = cmdNew(args)
	case "schema":
		err = cmdSchema(os.Stdout)
	case "init":
		err = cmdInit()
	case "doctor":
		code, err = cmdDoctor(ctx, args)
	case "hook":
		code = cmdHook(ctx, os.Stdin)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		code = 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "specval:", err)
		if code == 0 {
			code = 2
		}
	}
	os.Exit(code)
}

// parseArgs lets flags appear before or after positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

type FileReport struct {
	File     string    `json:"file"`
	Findings []Finding `json:"findings"`
}

type Report struct {
	Files    []FileReport `json:"files"`
	Errors   int          `json:"errors"`
	Warnings int          `json:"warnings"`
}

func (r *Report) add(file string, fs []Finding) {
	if fs == nil {
		fs = []Finding{}
	}
	r.Files = append(r.Files, FileReport{File: file, Findings: fs})
	for _, f := range fs {
		if f.Severity == "error" {
			r.Errors++
		} else {
			r.Warnings++
		}
	}
}

// run checks the registry plus the given spec files; paths are reported relative to base.
func run(ctx context.Context, cfg *Config, files []string, base string) (*Report, error) {
	eng, err := newEngine(ctx, cfg)
	if err != nil {
		return nil, err
	}
	rel := func(p string) string {
		if r, err := filepath.Rel(base, p); err == nil {
			return r
		}
		return p
	}
	r := &Report{}
	reg, err := eng.CheckRegistry(ctx)
	if err != nil {
		return nil, err
	}
	regFiles := make([]string, 0, len(reg))
	for f := range reg {
		regFiles = append(regFiles, f)
	}
	sort.Strings(regFiles)
	for _, f := range regFiles {
		r.add(rel(f), reg[f])
	}
	for _, f := range files {
		abs, _ := filepath.Abs(f)
		fs, err := eng.CheckSpec(ctx, abs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		r.add(rel(abs), fs)
	}
	return r, nil
}

func cmdCheck(ctx context.Context, args []string) (int, error) {
	fl := flag.NewFlagSet("check", flag.ContinueOnError)
	format := fl.String("format", "text", "output format: text or json")
	strict := fl.Bool("strict", false, "fail on warnings too")
	files, err := parseArgs(fl, args)
	if err != nil {
		return 2, err
	}
	cwd, _ := os.Getwd()
	start := cwd
	if len(files) > 0 {
		start = filepath.Dir(files[0])
	}
	cfg, err := findConfig(start)
	if err != nil {
		return 2, err
	}
	if len(files) == 0 {
		err := walkFiles(cfg.abs(cfg.SpecsDir), []string{".yaml", ".yml"}, func(p string) error {
			files = append(files, p)
			return nil
		})
		if err != nil {
			return 2, err
		}
	}
	r, err := run(ctx, cfg, files, cwd)
	if err != nil {
		return 2, err
	}
	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			return 2, err
		}
	case "text":
		printText(os.Stdout, r, len(files))
	default:
		return 2, fmt.Errorf("unknown format %q", *format)
	}
	if r.Errors > 0 || (*strict && r.Warnings > 0) {
		return 1, nil
	}
	return 0, nil
}

func printText(w io.Writer, r *Report, specs int) {
	for _, fr := range r.Files {
		for _, f := range fr.Findings {
			at := ""
			if f.Path != "" {
				at = " (" + f.Path + ")"
			}
			fmt.Fprintf(w, "%s:%d %s %s %s%s\n    fix: %s\n", fr.File, f.Line, f.Severity, f.Rule, f.Msg, at, f.Fix)
		}
	}
	fmt.Fprintf(w, "%d spec(s) checked: %d error(s), %d warning(s)\n", specs, r.Errors, r.Warnings)
}

// cmdHook handles a Claude Code PostToolUse event. Exit 2 sends stderr back to
// Claude; anything unrelated to specs exits 0 silently so the hook never gets in the way.
func cmdHook(ctx context.Context, stdin io.Reader) int {
	var in struct {
		Cwd       string `json:"cwd"`
		ToolInput struct {
			FilePath string `json:"file_path"`
		} `json:"tool_input"`
	}
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return 0
	}
	fp := in.ToolInput.FilePath
	if fp == "" || (filepath.Ext(fp) != ".yaml" && filepath.Ext(fp) != ".yml") {
		return 0
	}
	if !filepath.IsAbs(fp) {
		fp = filepath.Join(in.Cwd, fp)
	}
	cfg, err := findConfig(filepath.Dir(fp))
	if err != nil {
		return 0
	}
	if rel, err := filepath.Rel(cfg.abs(cfg.SpecsDir), fp); err != nil || strings.HasPrefix(rel, "..") {
		return 0
	}
	base := in.Cwd
	if base == "" {
		base = cfg.Root
	}
	r, err := run(ctx, cfg, []string{fp}, base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "specval:", err)
		return 1
	}
	if r.Errors > 0 {
		printText(os.Stderr, r, 1)
		fmt.Fprintln(os.Stderr, "The spec does not pass yet. Apply each fix above and save the spec again before writing any code.")
		return 2
	}
	if r.Warnings > 0 {
		var b strings.Builder
		printText(&b, r, 1)
		err := json.NewEncoder(os.Stdout).Encode(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     "PostToolUse",
				"additionalContext": "specval warnings (spec passes; consider fixing):\n" + b.String(),
			},
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "specval:", err)
			return 1
		}
	}
	return 0
}

func cmdSchema(w io.Writer) error {
	b, err := embedded.ReadFile("baseline/schema.json")
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func cmdContext(ctx context.Context, w io.Writer) error {
	cwd, _ := os.Getwd()
	cfg, err := findConfig(cwd)
	if err != nil {
		return err
	}
	eng, err := newEngine(ctx, cfg)
	if err != nil {
		return err
	}
	var dpl struct {
		LayerOrder []string            `json:"layer_order"`
		Layers     map[string][]string `json:"layers"`
		Suffix     map[string]string   `json:"suffix"`
	}
	var registry map[string]struct {
		Layer string `json:"layer"`
	}
	if err := roundTrip(eng.data["dpl"], &dpl); err != nil {
		return err
	}
	project, _ := eng.data["project"].(map[string]any)
	if err := roundTrip(project["services"], &registry); err != nil {
		return err
	}
	format, _ := embedded.ReadFile("baseline/format.yaml")

	fmt.Fprintf(w, "# Spec rules (specval)\n\nEvery change needs a passing spec at %s/<id>.yaml before code is written. Scaffold: `specval new <id>`. Validate: `specval check <file>`. Each finding includes a fix.\n\n", cfg.SpecsDir)
	fmt.Fprintf(w, "## Format\n```yaml\n%s```\n\n", format)

	fmt.Fprint(w, "## Layers (caller -> may call)\n")
	var suffixes []string
	for _, l := range dpl.LayerOrder {
		targets := strings.Join(dpl.Layers[l], ", ")
		if targets == "" {
			targets = "nothing"
		}
		fmt.Fprintf(w, "- %s -> %s\n", l, targets)
		suffixes = append(suffixes, dpl.Suffix[l])
	}
	fmt.Fprintf(w, "Service names end with their layer: %s.\n\n", strings.Join(suffixes, ", "))

	fmt.Fprint(w, "## Registered services\n")
	byLayer := map[string][]string{}
	for name, s := range registry {
		byLayer[s.Layer] = append(byLayer[s.Layer], name)
	}
	for _, l := range dpl.LayerOrder {
		if names := byLayer[l]; len(names) > 0 {
			sort.Strings(names)
			fmt.Fprintf(w, "- %s: %s\n", l, strings.Join(names, ", "))
		}
	}
	if len(registry) == 0 {
		fmt.Fprint(w, "(none yet)\n")
	}

	fmt.Fprint(w, "\n## Rules\n")
	for _, d := range eng.RuleDocs() {
		desc := ""
		if d.Description != "" {
			desc = ": " + d.Description
		}
		fmt.Fprintf(w, "- %s (%s) %s%s\n", d.ID, d.Severity, d.Title, desc)
	}
	return nil
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func cmdNew(args []string) error {
	fl := flag.NewFlagSet("new", flag.ContinueOnError)
	kind := fl.String("kind", "feature", "feature, bugfix, or chore")
	title := fl.String("title", "", "one-line title")
	pos, err := parseArgs(fl, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || !idRe.MatchString(pos[0]) {
		return errors.New("usage: specval new <id> (letters, digits, . _ -)")
	}
	cwd, _ := os.Getwd()
	cfg, err := findConfig(cwd)
	if err != nil {
		return err
	}
	path := filepath.Join(cfg.abs(cfg.SpecsDir), pos[0]+".yaml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	tmpl, _ := embedded.ReadFile("baseline/template.yaml")
	out := strings.NewReplacer("{{ID}}", pos[0], "{{KIND}}", *kind, "{{TITLE}}", strconv.Quote(*title)).Replace(string(tmpl))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}
