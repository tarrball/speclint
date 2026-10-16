// Package check finds the project of the specs, checks the specs and the
// registry with the policy engine, and collects the findings in a report.
// The sv check and sv hook commands use it.
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

// ErrNotSpec tells that a file is not a spec, because it is not in the specs
// directory of a specval project.
var ErrNotSpec = errors.New("the file is not a spec")

// FileReport holds the findings for one file.
type FileReport struct {
	File     string           `json:"file"`
	Findings []policy.Finding `json:"findings"`
}

// Report holds the findings of one check, and the totals. It is the JSON
// output of `sv check --format json`. Other people depend on that format.
type Report struct {
	Files    []FileReport `json:"files"`
	Errors   int          `json:"errors"`
	Warnings int          `json:"warnings"`
	// Specs is the number of specs that the check examined.
	Specs int `json:"-"`
}

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

// Run checks the registry and the given spec files of one project. dir is the
// working directory. A relative path in files is relative to dir, and the
// report gives each path relative to dir. If files is empty, Run checks all
// the specs of the project that contains dir. All files must be in the same
// project.
func Run(ctx context.Context, dir string, files []string) (*Report, error) {
	abs := make([]string, len(files))
	for i, f := range files {
		if filepath.IsAbs(f) {
			abs[i] = f
		} else {
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

// Spec checks one spec. It does not check the registry: a problem in the
// registry is not a problem in the spec. path must be an absolute path. The
// report gives the path relative to base. If base is "", the report gives the
// path relative to the project root. If the file is not in the specs
// directory of a project, Spec returns ErrNotSpec.
func Spec(ctx context.Context, path, base string) (*Report, error) {
	cfg, err := config.Find(filepath.Dir(path))
	if errors.Is(err, config.ErrNotFound) {
		return nil, ErrNotSpec
	} else if err != nil {
		// A configuration that specval cannot read must not let a spec pass
		// without a check. Thus this error is not ErrNotSpec.
		return nil, err
	}
	if rel, err := filepath.Rel(cfg.Abs(cfg.SpecsDir), path); err != nil || !filepath.IsLocal(rel) {
		return nil, ErrNotSpec
	}
	if base == "" {
		base = cfg.Root
	}
	return run(ctx, cfg, []string{path}, base, false)
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
