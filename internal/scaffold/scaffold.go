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

	"go.yaml.in/yaml/v3"

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

// Init creates each part of a specval project in dir that does not exist:
//
//   - the configuration file
//   - the data and policy files
//   - the specs directory
//   - the Claude Code skill
//   - the hook in the Claude Code settings
//
// Init does not change a file that exists. The one exception is the settings
// file, to which it adds the hook. Init writes one line to w for each part.
func Init(dir string, w io.Writer) error {
	// Read the settings file first, so that a file that is not valid stops
	// Init before it writes anything.
	settings := filepath.Join(dir, claude.SettingsPath)
	if _, err := claude.InspectSettings(settings); err != nil {
		return err
	}
	// A configuration in dir gives the paths for the other parts. Init does
	// not use the configuration of a parent directory.
	cfg := config.Default(dir)
	if _, err := os.Stat(filepath.Join(dir, config.FileName)); err == nil {
		if cfg, err = config.Find(dir); err != nil {
			return err
		}
	}
	files := []struct{ path, content string }{
		{config.FileName, configTemplate},
		{firstFile(cfg.Data, "services.yaml", ".yaml", ".yml", ".json"), servicesTemplate},
		{firstFile(cfg.Policy, "project.rego", ".rego"), projectTemplate},
		{claude.SkillPath, claude.Skill()},
	}
	for _, f := range files {
		if f.path == "" {
			continue
		}
		if err := writeIfMissing(dir, f.path, f.content, w); err != nil {
			return err
		}
	}
	specsDir := cfg.Abs(cfg.SpecsDir)
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return err
	}
	added, err := claude.AddHook(settings)
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

// firstFile returns the file that Init writes for the first of paths. If that
// path has one of exts, it is the file. If not, it is a directory, and the
// file is name in it. firstFile returns "" if there are no paths.
func firstFile(paths []string, name string, exts ...string) string {
	if len(paths) == 0 {
		return ""
	}
	for _, ext := range exts {
		if strings.HasSuffix(paths[0], ext) {
			return paths[0]
		}
	}
	return filepath.Join(paths[0], name)
}

// writeIfMissing writes content to path, relative to dir, if the file does
// not exist. It writes one line about the file to w.
func writeIfMissing(dir, path, content string, w io.Writer) error {
	full := path
	if !filepath.IsAbs(path) {
		full = filepath.Join(dir, path)
	}
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
