// Package scaffold writes the files of a new specval project and new specs.
package scaffold

import (
	_ "embed" // The templates are embedded.
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/tarrball/specval/internal/baseline"
	"github.com/tarrball/specval/internal/claude"
	"github.com/tarrball/specval/internal/config"
)

var (
	//go:embed templates/config.yaml
	configTemplate string

	//go:embed templates/services.yaml
	servicesTemplate string

	//go:embed templates/project.rego
	projectTemplate string

	//go:embed templates/spec.yaml
	specTemplate string
)

// Init creates the parts of a specval project in dir that do not exist yet:
// the configuration, the data and policy files, the specs directory, the
// Claude Code skill, and the hook. It does not change a file that exists,
// except that it adds the hook to an existing settings file. Init writes one
// line to w for each part.
func Init(dir string, w io.Writer) error {
	specsDir := filepath.Join(dir, "specs")
	if cfg, err := config.Find(dir); err == nil && sameDir(cfg.Root, dir) {
		specsDir = cfg.Abs(cfg.SpecsDir)
	}
	files := []struct{ path, content string }{
		{config.FileName, configTemplate},
		{".specval/data/services.yaml", servicesTemplate},
		{".specval/policy/project.rego", projectTemplate},
		{claude.SkillPath, claude.Skill()},
	}
	for _, f := range files {
		if err := writeIfMissing(dir, f.path, f.content, w); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return err
	}
	added, err := claude.AddHook(filepath.Join(dir, claude.SettingsPath))
	if err != nil {
		return err
	}
	if added {
		_, err = fmt.Fprintln(w, "hooked  ", claude.SettingsPath)
	} else {
		_, err = fmt.Fprintln(w, "exists  ", claude.SettingsPath, "(specval hook)")
	}
	return err
}

func writeIfMissing(dir, path, content string, w io.Writer) error {
	full := filepath.Join(dir, path)
	if _, err := os.Stat(full); err == nil {
		_, err = fmt.Fprintln(w, "exists  ", path)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "created ", path)
	return err
}

// quote returns s as a YAML double-quoted string.
func quote(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("the text is not valid UTF-8")
	}
	b, err := yaml.Marshal(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s, Style: yaml.DoubleQuotedStyle})
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(b), "\n"), nil
}

func sameDir(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return a == b
}

// ValidID reports whether id can be the ID of a spec.
func ValidID(id string) bool {
	return baseline.IDPattern().MatchString(id)
}

// Spec writes a new spec, made from the template, to <specs dir>/<id>.yaml.
// It returns the path of the spec. It does not overwrite a file. The caller
// must make sure that id is valid and that kind is one of baseline.Kinds.
func Spec(cfg *config.Config, id, kind, title string) (string, error) {
	path := filepath.Join(cfg.Abs(cfg.SpecsDir), id+".yaml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	}
	quoted, err := quote(title)
	if err != nil {
		return "", fmt.Errorf("title: %w", err)
	}
	out := strings.NewReplacer("{{ID}}", id, "{{KIND}}", kind, "{{TITLE}}", quoted).Replace(specTemplate)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
