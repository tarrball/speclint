// Package check checks the registry and the spec files of a specval project,
// and builds a report of the findings. It is the manager of sv check and sv hook.
package check

import (
	"context"
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
}

// Report holds the findings of one check, and the totals.
type Report struct {
	Files    []FileReport `json:"files"`
	Errors   int          `json:"errors"`
	Warnings int          `json:"warnings"`
	// Specs is the number of spec files that Check checked. The JSON output
	// does not include it.
	Specs int `json:"-"`
}

// Add adds the findings for file to the report, and adds them to the totals.
func (r *Report) Add(file string, found []policy.Finding) {
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

// Request tells Check what to check.
type Request struct {
	// Config is the configuration of the project. If it is nil, Check finds
	// the configuration in the directory of the first file, or in Dir if
	// there are no files.
	Config *config.Config
	// Files are the spec files to check. A relative path is relative to Dir.
	// If there are no files, Check checks each spec in the specs directory.
	Files []string
	// Dir is the working directory, as an absolute path. The report gives
	// paths relative to Dir.
	Dir string
	// Registry is true if Check must also check the registry.
	Registry bool
}

// Check checks the spec files of req and, if req.Registry is true, the
// registry. All files must be in one project. It returns an error only if it
// cannot do the check. A finding is not an error.
func Check(ctx context.Context, req Request) (*Report, error) {
	files := make([]string, len(req.Files))
	for i, f := range req.Files {
		if !filepath.IsAbs(f) {
			f = filepath.Join(req.Dir, f)
		}
		files[i] = f
	}
	cfg := req.Config
	if cfg == nil {
		start := req.Dir
		if len(files) > 0 {
			start = filepath.Dir(files[0])
		}
		var err error
		if cfg, err = config.Find(start); err != nil {
			return nil, err
		}
	}
	if err := sameProject(cfg, files, req.Dir); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		var err error
		if files, err = cfg.SpecFiles(); err != nil {
			return nil, err
		}
	}
	eng, err := policy.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	rel := func(p string) string { return relTo(req.Dir, p) }
	r := &Report{Files: []FileReport{}, Specs: len(files)}
	if req.Registry {
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
			r.Add(rel(f), reg[f])
		}
	}
	for _, f := range files {
		found, err := eng.CheckSpec(ctx, f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel(f), err)
		}
		r.Add(rel(f), found)
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
