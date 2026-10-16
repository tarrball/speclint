// Package check checks the specs and the registry of a project for the check
// and hook commands. It finds the configuration, loads the rule engine, and
// builds the report.
package check

import (
	"context"
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
	// Specs is the number of spec files that the check examined. The
	// registry is not a spec.
	Specs int `json:"-"`
}

// add adds the findings for file, and adds them to the totals.
func (r *Report) add(file string, found []policy.Finding) {
	if found == nil {
		found = []policy.Finding{}
	}
	r.Files = append(r.Files, FileReport{File: file, Findings: found})
	for _, f := range found {
		switch f.Severity {
		case policy.SeverityError:
			r.Errors++
		case policy.SeverityWarning:
			r.Warnings++
		}
	}
}

// WriteText writes each finding and the totals to w as text.
func (r *Report) WriteText(w io.Writer) {
	for _, fr := range r.Files {
		for _, f := range fr.Findings {
			where := fr.File
			if f.Line > 0 {
				where = fmt.Sprintf("%s:%d", fr.File, f.Line)
			}
			at := ""
			if f.Path != "" {
				at = " (" + f.Path + ")"
			}
			fmt.Fprintf(w, "%s %s %s %s%s\n    fix: %s\n", where, f.Severity, f.Rule, f.Msg, at, f.Fix)
		}
	}
	fmt.Fprintf(w, "specval checked %d spec(s) and found %d error(s) and %d warning(s).\n", r.Specs, r.Errors, r.Warnings)
}

// Project checks the registry and the given spec files. If files is empty, it
// checks each spec in the specs directory. dir is the working directory, as
// an absolute path. Project reads a relative file from dir, and reports paths
// relative to dir. It finds the configuration from the directory of the first
// file, or from dir. All files must be in the same project.
func Project(ctx context.Context, dir string, files []string) (*Report, error) {
	abs := make([]string, len(files))
	for i, f := range files {
		abs[i] = f
		if !filepath.IsAbs(f) {
			abs[i] = filepath.Join(dir, f)
		}
	}
	start := dir
	if len(abs) > 0 {
		start = filepath.Dir(abs[0])
	}
	cfg, err := config.Find(start)
	if err != nil {
		return nil, err
	}
	if err := sameProject(cfg, abs, dir); err != nil {
		return nil, err
	}
	if len(abs) == 0 {
		if abs, err = cfg.SpecFiles(); err != nil {
			return nil, err
		}
	}
	return run(ctx, cfg, abs, dir, true)
}

// run checks the given spec files and, if registry is true, the registry. The
// spec files must be absolute paths. run reports paths relative to base.
func run(ctx context.Context, cfg *config.Config, files []string, base string, registry bool) (*Report, error) {
	eng, err := policy.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	rel := func(p string) string { return relTo(base, p) }
	r := &Report{Files: []FileReport{}, Specs: len(files)}
	if registry {
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
	}
	for _, f := range files {
		found, err := eng.CheckSpec(ctx, f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel(f), err)
		}
		r.add(rel(f), found)
	}
	return r, nil
}

// sameProject returns an error if a file is not in the project of cfg. The
// rules and registry of one project do not apply to the specs of another.
func sameProject(cfg *config.Config, files []string, base string) error {
	for _, f := range files[min(1, len(files)):] {
		other, err := config.Find(filepath.Dir(f))
		if err != nil {
			return err
		}
		if !sameDir(other.Root, cfg.Root) {
			return fmt.Errorf("%s and %s are in different specval projects; check each project separately",
				relTo(base, files[0]), relTo(base, f))
		}
	}
	return nil
}

// sameDir reports whether a and b are the same directory. A symbolic link to
// a directory is the same directory.
func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}

// relTo returns p relative to base. If that is not possible, it returns p.
func relTo(base, p string) string {
	if r, err := filepath.Rel(base, p); err == nil {
		return r
	}
	return p
}
