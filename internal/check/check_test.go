package check

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tarrball/specval/internal/policy"
)

// refund returns the absolute path of the example project.
func refund(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples", "refund"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// project makes a project with an empty configuration file in a temporary
// directory and returns the directory.
func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".specval.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReportAddCountsFindings(t *testing.T) {
	r := &Report{}
	r.add("a.yaml", nil)
	r.add("b.yaml", []policy.Finding{
		{Rule: "SV101", Severity: policy.SeverityError},
		{Rule: "SV201", Severity: policy.SeverityWarning},
	})
	if r.Errors != 1 || r.Warnings != 1 || len(r.Files) != 2 || r.Files[0].Findings == nil {
		t.Errorf("report = %+v", r)
	}
}

func TestRunChecksAllSpecsWhenNoFilesAreGiven(t *testing.T) {
	dir := refund(t)
	r, err := Run(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Specs != 3 || r.Errors == 0 {
		t.Errorf("report = %+v", r)
	}
	for _, fr := range r.Files {
		if filepath.IsAbs(fr.File) {
			t.Errorf("file %s is not relative to %s", fr.File, dir)
		}
	}
}

func TestRunResolvesRelativePathsFromDir(t *testing.T) {
	r, err := Run(context.Background(), refund(t), []string{"specs/ORD-142.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Specs != 1 || r.Errors != 0 || len(r.Files) != 1 || r.Files[0].File != filepath.Join("specs", "ORD-142.yaml") {
		t.Errorf("report = %+v", r)
	}
}

func TestRunRejectsSpecsFromTwoProjects(t *testing.T) {
	a, b := project(t), project(t)
	_, err := Run(context.Background(), a, []string{
		filepath.Join(a, "specs", "x.yaml"),
		filepath.Join(b, "specs", "y.yaml"),
	})
	if err == nil {
		t.Error("Run accepted specs from two projects")
	}
}

func TestSpecReturnsErrNotSpecForAFileThatIsNotASpec(t *testing.T) {
	dir := project(t)
	for _, p := range []string{
		filepath.Join(t.TempDir(), "specs", "a.yaml"), // not in a project
		filepath.Join(dir, "other", "a.yaml"),         // not in the specs directory
	} {
		if _, err := Spec(context.Background(), p, dir); !errors.Is(err, ErrNotSpec) {
			t.Errorf("%s: err = %v, want ErrNotSpec", p, err)
		}
	}
}

func TestSpecReturnsTheErrorOfAConfigurationThatIsNotValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".specval.yaml"), []byte("specs_dir: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Spec(context.Background(), filepath.Join(dir, "specs", "a.yaml"), dir)
	if err == nil || errors.Is(err, ErrNotSpec) {
		t.Errorf("err = %v, want a configuration error", err)
	}
}

// badRegistry makes a project with one spec and a registry that has a
// finding. It returns the directory and the path of the spec.
func badRegistry(t *testing.T) (dir, spec string) {
	t.Helper()
	dir = project(t)
	data := filepath.Join(dir, ".specval", "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "services.yaml"), []byte("services:\n  X: { layer: nope }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec = filepath.Join(dir, "specs", "a.yaml")
	if err := os.WriteFile(spec, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, spec
}

func TestRunChecksTheRegistry(t *testing.T) {
	dir, spec := badRegistry(t)
	r, err := Run(context.Background(), dir, []string{spec})
	if err != nil {
		t.Fatal(err)
	}
	if r.Specs != 1 || len(r.Files) != 2 || r.Files[0].File != filepath.Join(".specval", "data", "services.yaml") {
		t.Errorf("report = %+v", r)
	}
}

func TestSpecDoesNotCheckTheRegistry(t *testing.T) {
	dir, spec := badRegistry(t)
	r, err := Spec(context.Background(), spec, "")
	if err != nil {
		t.Fatal(err)
	}
	// With no base, the path is relative to the project root.
	if r.Specs != 1 || len(r.Files) != 1 || r.Files[0].File != filepath.Join("specs", "a.yaml") {
		t.Errorf("report = %+v (project %s)", r, dir)
	}
}
