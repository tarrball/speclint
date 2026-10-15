// Package scaffold writes the files of a new specval project and new specs.
package scaffold

import (
	_ "embed" // The templates are embedded.
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

	//go:embed templates/services.json
	servicesJSONTemplate string

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
// file, to which it adds the hook. Init writes one line to w for each file.
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
	// A data file can already define the registry. Then Init must not write a
	// second one, because two files cannot define the same key.
	registry, err := registryFile(cfg)
	if err != nil {
		return err
	}
	services := firstFile(cfg, cfg.Data, "services.yaml", ".yaml", ".yml", ".json")
	servicesContent := servicesTemplate
	if strings.HasSuffix(services, ".json") {
		servicesContent = servicesJSONTemplate
	}
	if registry != "" {
		services, servicesContent = registry, ""
	}
	files := []struct{ path, content string }{
		{config.FileName, configTemplate},
		{services, servicesContent},
		{firstFile(cfg, cfg.Policy, "project.rego", ".rego"), projectTemplate},
		{claude.SkillPath, claude.Skill(cfg.SpecsDir)},
	}
	for _, f := range files {
		switch f.path {
		case "":
			continue
		case registry:
			if _, err := fmt.Fprintln(w, "exists  ", relPath(dir, registry), "(services)"); err != nil {
				return err
			}
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
// path is a directory, the file is name in it. If it is a file, it is the
// file. If it does not exist, firstFile uses its extension: with one of exts,
// it is the file, and without one, it is a directory. firstFile returns "" if
// there are no paths.
func firstFile(cfg *config.Config, paths []string, name string, exts ...string) string {
	if len(paths) == 0 {
		return ""
	}
	p := paths[0]
	if info, err := os.Stat(cfg.Abs(p)); err == nil {
		if info.IsDir() {
			return filepath.Join(p, name)
		}
		return p
	}
	for _, ext := range exts {
		if strings.HasSuffix(p, ext) {
			return p
		}
	}
	return filepath.Join(p, name)
}

// registryFile returns the project data file that defines services, or "" if
// no data file does.
func registryFile(cfg *config.Config) (string, error) {
	files, err := cfg.DataFiles()
	if err != nil {
		return "", err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		var doc map[string]any
		// Init does not check the data files. If a file is not valid, check
		// reports it, and Init looks at the next file.
		if yaml.Unmarshal(b, &doc) != nil {
			continue
		}
		if _, ok := doc["services"]; ok {
			return f, nil
		}
	}
	return "", nil
}

// relPath returns p relative to dir, if that is possible.
func relPath(dir, p string) string {
	if r, err := filepath.Rel(dir, p); err == nil {
		return r
	}
	return p
}

// writeIfMissing writes content to path, relative to dir, if the file does
// not exist. It writes one line about the file to w.
func writeIfMissing(dir, path, content string, w io.Writer) error {
	full := path
	if !filepath.IsAbs(path) {
		full = filepath.Join(dir, path)
	}
	created, err := createFile(full, content)
	if err != nil {
		return err
	}
	if created {
		_, err = fmt.Fprintln(w, "created ", path)
	} else {
		_, err = fmt.Fprintln(w, "exists  ", path)
	}
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
// It returns the path of the spec. It does not overwrite a file.
func Spec(cfg *config.Config, id, kind, title string) (string, error) {
	// A valid ID cannot contain a path separator, so the file is in the specs
	// directory.
	if !ValidID(id) {
		return "", fmt.Errorf("the id %q is not valid; it must start with a letter or a digit, and contain only letters, digits, '.', '_', and '-'", id)
	}
	if kinds := baseline.Kinds(); !slices.Contains(kinds, kind) {
		return "", fmt.Errorf("the kind %q is not valid; use one of: %s", kind, strings.Join(kinds, ", "))
	}
	quoted, err := quote(title)
	if err != nil {
		return "", fmt.Errorf("title: %w", err)
	}
	out := strings.NewReplacer("{{ID}}", id, "{{KIND}}", kind, "{{TITLE}}", quoted).Replace(specTemplate)
	path := filepath.Join(cfg.Abs(cfg.SpecsDir), id+".yaml")
	created, err := createFile(path, out)
	if err != nil {
		return "", err
	}
	if !created {
		return "", fmt.Errorf("%s already exists", path)
	}
	return path, nil
}

// createFile writes content to a new file at path, and makes its parent
// directories. If the file exists, createFile does not change it, and returns
// false. It checks and creates the file in one step, so that it cannot
// overwrite a file that another process creates at the same time.
func createFile(path, content string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close() // The write error is the one to report.
		return false, err
	}
	return true, f.Close()
}
