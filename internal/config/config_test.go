package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeFiles creates each file in files under dir. The map key is a path
// relative to dir, and the value is the content.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFindUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{FileName: ""})
	c, err := Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Root != dir || c.SpecsDir != "specs" ||
		!slices.Equal(c.Policy, []string{".specval/policy"}) || !slices.Equal(c.Data, []string{".specval/data"}) {
		t.Errorf("Find = %+v, want the defaults with Root %s", c, dir)
	}
}

func TestFindReadsValuesFromAParentDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		FileName:        "specs_dir: changes\npolicy: [rules]\ndata: [a.yaml, b]\n",
		"sub/sub/.keep": "",
	})
	c, err := Find(filepath.Join(dir, "sub", "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Root != dir || c.SpecsDir != "changes" ||
		!slices.Equal(c.Policy, []string{"rules"}) || !slices.Equal(c.Data, []string{"a.yaml", "b"}) {
		t.Errorf("Find = %+v", c)
	}
}

func TestFindErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Find(dir); err == nil || !strings.Contains(err.Error(), "no .specval.yaml found") {
		t.Errorf("Find with no config: err = %v", err)
	}
	writeFiles(t, dir, map[string]string{FileName: "specs_dir: [unclosed\n"})
	if _, err := Find(dir); err == nil || !strings.Contains(err.Error(), filepath.Join(dir, FileName)) {
		t.Errorf("Find with bad YAML: err = %v, want the path in it", err)
	}
}

func TestAbs(t *testing.T) {
	c := &Config{Root: filepath.FromSlash("/project")}
	if got, want := c.Abs("specs"), filepath.FromSlash("/project/specs"); got != want {
		t.Errorf("Abs(relative) = %s, want %s", got, want)
	}
	abs := filepath.FromSlash("/elsewhere/specs")
	if got := c.Abs(abs); got != abs {
		t.Errorf("Abs(absolute) = %s, want %s", got, abs)
	}
}

func TestFileLists(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"specs/b.yaml":           "",
		"specs/a.yml":            "",
		"specs/sub/c.yaml":       "",
		"specs/notes.md":         "",
		"data/services.yaml":     "",
		"data/limits.json":       "",
		"data/readme.txt":        "",
		"extra.txt":              "",
		"policy/rules.rego":      "",
		"policy/rules_test.rego": "",
		"one_test.rego":          "",
	})
	c := &Config{
		Root:     dir,
		SpecsDir: "specs",
		// A listed file is used even if its extension is not one of the
		// usual extensions. A path that does not exist is ignored.
		Data:   []string{"data", "extra.txt", "missing"},
		Policy: []string{"policy", "one_test.rego"},
	}
	for _, tc := range []struct {
		name string
		list func() ([]string, error)
		want []string
	}{
		{"SpecFiles", c.SpecFiles, []string{"specs/a.yml", "specs/b.yaml", "specs/sub/c.yaml"}},
		{"DataFiles", c.DataFiles, []string{"data/limits.json", "data/services.yaml", "extra.txt"}},
		{"PolicyFiles", c.PolicyFiles, []string{"policy/rules.rego"}},
	} {
		got, err := tc.list()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var want []string
		for _, w := range tc.want {
			want = append(want, filepath.Join(dir, filepath.FromSlash(w)))
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", tc.name, got, want)
		}
	}
}

func TestSpecFilesWithNoSpecsDir(t *testing.T) {
	c := &Config{Root: t.TempDir(), SpecsDir: "specs"}
	got, err := c.SpecFiles()
	if err != nil || len(got) != 0 {
		t.Errorf("SpecFiles = %v, %v; want no files and no error", got, err)
	}
}
