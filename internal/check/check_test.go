package check

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tarrball/specval/internal/config"
	"github.com/tarrball/specval/internal/policy"
)

const registry = `services:
  OrderManager: { layer: manager }
  OrderAccessor: { layer: accessor }
`

// spec returns a spec in which OrderManager calls the service to.
func spec(to string) string {
	return `specval: 1
id: A-1
kind: bugfix
title: T
summary: OrderManager reads an order.
acceptance:
  - { id: AC1, text: "Given an order, when it is read, then it is returned." }
calls:
  - { from: OrderManager, to: ` + to + `, op: GetOrder, covers: [AC1] }
`
}

// project writes a .specval.yaml and the given files to dir, and returns
// dir. The map key is a path relative to dir, and the value is the content.
func project(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	files[config.FileName] = ""
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fileNames returns the file of each file report in r.
func fileNames(r *Report) []string {
	var names []string
	for _, fr := range r.Files {
		names = append(names, fr.File)
	}
	return names
}

func TestReportAdd(t *testing.T) {
	r := &Report{}
	r.Add("a.yaml", nil)
	r.Add("b.yaml", []policy.Finding{
		{Rule: "SV101", Severity: policy.SeverityError},
		{Rule: "SV201", Severity: policy.SeverityWarning},
	})
	if r.Errors != 1 || r.Warnings != 1 || len(r.Files) != 2 || r.Files[0].Findings == nil {
		t.Errorf("report = %+v", r)
	}
}

func TestCheckEachSpecInTheSpecsDir(t *testing.T) {
	dir := project(t, t.TempDir(), map[string]string{
		".specval/data/services.yaml": registry,
		"specs/a.yaml":                spec("OrderAccessor"),
		"specs/b.yaml":                spec("Nobody"),
	})
	r, err := Check(context.Background(), Request{Dir: dir, Registry: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join("specs", "a.yaml"), filepath.Join("specs", "b.yaml")}
	if r.Specs != 2 || !slices.Equal(fileNames(r), want) || r.Errors == 0 {
		t.Errorf("report = %+v; want the files %v and an error", r, want)
	}
}

func TestCheckRelativeFile(t *testing.T) {
	dir := project(t, t.TempDir(), map[string]string{
		".specval/data/services.yaml": registry,
		"specs/a.yaml":                spec("OrderAccessor"),
		"specs/b.yaml":                spec("Nobody"),
	})
	sub := filepath.Join(dir, "specs")
	r, err := Check(context.Background(), Request{Files: []string{"b.yaml"}, Dir: sub})
	if err != nil {
		t.Fatal(err)
	}
	if r.Specs != 1 || !slices.Equal(fileNames(r), []string{"b.yaml"}) || r.Errors == 0 {
		t.Errorf("report = %+v; want only b.yaml, with an error", r)
	}
}

func TestCheckRegistry(t *testing.T) {
	dir := project(t, t.TempDir(), map[string]string{
		".specval/data/services.yaml": "services:\n  OrderManager: manager\n",
		"specs/.keep":                 "",
	})
	reg := filepath.Join(".specval", "data", "services.yaml")
	for _, withRegistry := range []bool{true, false} {
		r, err := Check(context.Background(), Request{Dir: dir, Registry: withRegistry})
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(fileNames(r), reg); got != withRegistry {
			t.Errorf("Registry %v: report = %+v", withRegistry, r)
		}
	}
}

func TestCheckUsesTheGivenConfig(t *testing.T) {
	root := t.TempDir()
	a := project(t, filepath.Join(root, "a"), map[string]string{
		".specval/data/services.yaml": registry + "  ShipAccessor: { layer: accessor }\n",
	})
	b := project(t, filepath.Join(root, "b"), map[string]string{
		".specval/data/services.yaml": registry,
		"specs/x.yaml":                spec("ShipAccessor"),
	})
	file := filepath.Join(b, "specs", "x.yaml")
	cfg, err := config.Find(a)
	if err != nil {
		t.Fatal(err)
	}
	// The registry of a has ShipAccessor. The registry of b does not.
	r, err := Check(context.Background(), Request{Config: cfg, Files: []string{file}, Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	if r.Errors != 0 {
		t.Errorf("with the configuration of a: report = %+v; want no errors", r)
	}
	r, err = Check(context.Background(), Request{Files: []string{file}, Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	if r.Errors == 0 {
		t.Errorf("with no configuration: report = %+v; want the error of the registry of b", r)
	}
}

func TestCheckFilesInDifferentProjects(t *testing.T) {
	root := t.TempDir()
	project(t, filepath.Join(root, "a"), map[string]string{"specs/x.yaml": spec("OrderAccessor")})
	project(t, filepath.Join(root, "b"), map[string]string{"specs/y.yaml": spec("OrderAccessor")})
	_, err := Check(context.Background(), Request{Files: []string{"a/specs/x.yaml", "b/specs/y.yaml"}, Dir: root})
	if err == nil || !strings.Contains(err.Error(), "a/specs/x.yaml and b/specs/y.yaml are in different specval projects") {
		t.Errorf("err = %v", err)
	}
}

func TestCheckWithoutAProject(t *testing.T) {
	_, err := Check(context.Background(), Request{Dir: t.TempDir()})
	if !errors.Is(err, config.ErrNotFound) {
		t.Errorf("err = %v; want %v", err, config.ErrNotFound)
	}
}

func TestSameDir(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip(err)
	}
	if !sameDir(dir, link) || sameDir(dir, t.TempDir()) {
		t.Error("sameDir does not compare the directories correctly")
	}
}
