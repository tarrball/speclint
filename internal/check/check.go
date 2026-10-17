// Package check is CheckManager. It finds the configuration of a project,
// lists its specs, checks the registry and the specs with the policy engine,
// and makes the report. It does not read or write stdio. The commands write
// the report.
package check

import (
	"context"
	"errors"
	"fmt"
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
	// Stopped is true if the spec has format errors, and thus the other
	// rules did not run.
	Stopped bool `json:"-"`
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

// add adds the result for file, and adds its findings to the totals.
func (r *Report) add(file string, res policy.SpecResult) {
	found := res.Findings
	if found == nil {
		found = []policy.Finding{}
	}
	r.Files = append(r.Files, FileReport{File: file, Findings: found, Stopped: res.Stopped})
	for _, f := range found {
		switch f.Severity {
		case policy.SeverityError:
			r.Errors++
		case policy.SeverityWarning:
			r.Warnings++
		}
	}
}

// Project checks the registry and the given spec files. If files is empty, it
// checks each spec in the specs directory. dir is the working directory, as an
// absolute path. Project reads a relative file from dir, and reports paths
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

// SpecFile checks one file if it is a spec: a file in the specs directory of
// a project. path must be absolute. SpecFile reports paths relative to base,
// or relative to the project root if base is empty.
// It does not check the registry. A problem in the registry is not a problem
// in the spec.
//
// If the file is not in a project or not in the specs directory, SpecFile
// returns a nil report and no error. If it cannot read the configuration of
// the project, it returns the error, so that a spec does not pass without a
// check.
func SpecFile(ctx context.Context, path, base string) (*Report, error) {
	cfg, err := config.Find(filepath.Dir(path))
	if errors.Is(err, config.ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if !inDir(cfg.Abs(cfg.SpecsDir), path) {
		return nil, nil
	}
	if base == "" {
		base = cfg.Root
	}
	return run(ctx, cfg, []string{path}, base, false)
}

// run checks the given spec files and, if registry is true, the registry. The
// spec files must be absolute paths. run reports paths relative to base.
func run(ctx context.Context, cfg *config.Config, files []string, base string, registry bool) (*Report, error) {
	src, err := cfg.ReadSources()
	if err != nil {
		return nil, err
	}
	eng, err := policy.New(ctx, src.Data, src.Policies)
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
			r.add(rel(f), policy.SpecResult{Findings: reg[f]})
		}
	}
	for _, f := range files {
		b, err := config.ReadSpec(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel(f), err)
		}
		res, err := eng.CheckSpec(ctx, b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel(f), err)
		}
		r.add(rel(f), res)
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

// inDir reports whether path is in dir or in a directory below dir.
func inDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}

// relTo returns p relative to base. If that is not possible, it returns p.
func relTo(base, p string) string {
	if r, err := filepath.Rel(base, p); err == nil {
		return r
	}
	return p
}
