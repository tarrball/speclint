package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/tarrball/specval/internal/config"
	"github.com/tarrball/specval/internal/policy"
)

// FileReport holds the findings for one file.
type FileReport struct {
	File     string           `json:"file"`
	Findings []policy.Finding `json:"findings"`
}

// Report holds the findings of one check, and the totals.
type Report struct {
	Files    []FileReport `json:"files"`
	Errors   int          `json:"errors"`
	Warnings int          `json:"warnings"`
}

func (r *Report) add(file string, found []policy.Finding) {
	if found == nil {
		found = []policy.Finding{}
	}
	r.Files = append(r.Files, FileReport{File: file, Findings: found})
	for _, f := range found {
		if f.Severity == "error" {
			r.Errors++
		} else {
			r.Warnings++
		}
	}
}

// run checks the registry and the given spec files. It reports paths
// relative to base.
func run(ctx context.Context, cfg *config.Config, files []string, base string) (*Report, error) {
	eng, err := policy.New(ctx, cfg)
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
		abs, err := filepath.Abs(f)
		if err != nil {
			return nil, err
		}
		found, err := eng.CheckSpec(ctx, abs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		r.add(rel(abs), found)
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
	cwd, err := os.Getwd()
	if err != nil {
		return 2, err
	}
	start := cwd
	if len(files) > 0 {
		start = filepath.Dir(files[0])
	}
	cfg, err := config.Find(start)
	if err != nil {
		return 2, err
	}
	if len(files) == 0 {
		if files, err = cfg.SpecFiles(); err != nil {
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
