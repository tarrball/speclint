package check

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tarrball/speclint/internal/policy"
)

// project writes the files of a project in a new directory, and returns the
// directory.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const badSpec = `speclint: 1
id: A
kind: chore
title: T
summary: S
acceptance: [{ id: AC1, text: T }]
calls: [{ from: OrderManager, to: Nobody, op: Get, covers: [AC1] }]
`

func TestProjectReportsTheRegistryAndTheSpecs(t *testing.T) {
	dir := project(t, map[string]string{
		".speclint.yaml":               "",
		".speclint/data/services.yaml": "services:\n  OrderManager: { layer: manager }\n  Order: { layer: accessor }\n",
		"specs/a.yaml":                 badSpec,
	})
	r, err := Project(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Specs != 1 || r.Errors != 2 || r.Warnings != 0 || len(r.Files) != 2 {
		t.Fatalf("report = %+v", r)
	}
	reg, spec := r.Files[0], r.Files[1]
	if reg.File != filepath.Join(".speclint", "data", "services.yaml") || len(reg.Findings) != 1 || reg.Findings[0].Rule != "SR002" {
		t.Errorf("registry report = %+v", reg)
	}
	if spec.File != filepath.Join("specs", "a.yaml") || len(spec.Findings) != 1 || spec.Findings[0].Rule != "SV101" {
		t.Errorf("spec report = %+v", spec)
	}
}

func TestAddGivesAnEmptyListForAFileWithoutFindings(t *testing.T) {
	// The JSON output gives "findings": [] for such a file, not null.
	r := &Report{}
	r.add("a.yaml", policy.SpecResult{})
	if len(r.Files) != 1 || r.Files[0].File != "a.yaml" || r.Files[0].Findings == nil || len(r.Files[0].Findings) != 0 {
		t.Errorf("report = %+v", r)
	}
	if r.Errors != 0 || r.Warnings != 0 {
		t.Errorf("totals = %d errors, %d warnings; want 0, 0", r.Errors, r.Warnings)
	}
}

func TestSpecFile(t *testing.T) {
	dir := project(t, map[string]string{
		".speclint.yaml":               "",
		".speclint/data/services.yaml": "services:\n  OrderManager: { layer: manager }\n  Order: { layer: accessor }\n",
		"specs/a.yaml":                 badSpec,
		"other/b.yaml":                 badSpec,
	})
	ctx := context.Background()

	// The hook reports only the findings of the spec, not the registry.
	r, err := SpecFile(ctx, filepath.Join(dir, "specs", "a.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || r.Specs != 1 || r.Errors != 1 || len(r.Files) != 1 || r.Files[0].File != filepath.Join("specs", "a.yaml") {
		t.Fatalf("report = %+v", r)
	}

	// A file that is not in the specs directory is not a spec.
	if r, err := SpecFile(ctx, filepath.Join(dir, "other", "b.yaml"), dir); r != nil || err != nil {
		t.Errorf("file outside specs: report %+v, err %v", r, err)
	}

	// A file that is not in a project is not a spec.
	alone := t.TempDir()
	if r, err := SpecFile(ctx, filepath.Join(alone, "b.yaml"), alone); r != nil || err != nil {
		t.Errorf("file outside a project: report %+v, err %v", r, err)
	}

	// A configuration that speclint cannot read is an error. The spec must
	// not pass without a check.
	bad := project(t, map[string]string{".speclint.yaml": "specs: [\n", "specs/a.yaml": badSpec})
	if _, err := SpecFile(ctx, filepath.Join(bad, "specs", "a.yaml"), bad); err == nil {
		t.Error("bad configuration: no error")
	}
}

func TestProjectRefusesSpecsOfTwoProjects(t *testing.T) {
	files := map[string]string{".speclint.yaml": "", ".speclint/data/services.yaml": "services: {}\n", "specs/a.yaml": badSpec}
	one, two := project(t, files), project(t, files)
	_, err := Project(context.Background(), one, []string{"specs/a.yaml", filepath.Join(two, "specs", "a.yaml")})
	if err == nil {
		t.Fatal("no error")
	}
}
