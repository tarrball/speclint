// Package scaffold writes the files of a new specval project and new specs.
package scaffold

import (
	_ "embed" // For the go:embed directives.
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
	"github.com/tarrball/specval/internal/copilot"
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
//   - the Copilot review instructions
//   - the hook in the Claude Code settings
//
// Init does not change a file that exists. The one exception is the settings
// file, to which it adds the hook. If the policy paths have a Rego file, Init
// does not add the example rule file. Init writes one line to w for each
// file. If no CLAUDE.md tells about specval, Init then writes a line for the
// user to add to it.
func Init(dir string, w io.Writer) error { return setUp(dir, w, false) }

// DryRun writes the lines of Init to w, with "create" and "hook" for the
// changes that Init makes. It gives the same errors as Init, and it writes
// no file and no directory.
func DryRun(dir string, w io.Writer) error { return setUp(dir, w, true) }

// setUp does the work of Init, or of DryRun if dry is true.
func setUp(dir string, w io.Writer, dry bool) error {
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
	registryPath := relPath(dir, services)
	if services == "" {
		registryPath = "the data files that " + config.FileName + " lists"
	}
	// A project with its own rules does not need the example rule file.
	rules, err := cfg.PolicyFiles()
	if err != nil {
		return err
	}
	policy := firstFile(cfg, cfg.Policy, "project.rego", ".rego")
	if len(rules) > 0 {
		policy = rules[0]
	}
	files := []struct{ path, content string }{
		{config.FileName, configTemplate},
		{services, servicesContent},
		{policy, projectTemplate},
		{claude.SkillPath, claude.Skill(cfg.SpecsDir)},
		{copilot.InstructionsPath, copilot.Instructions(cfg.SpecsDir, registryPath)},
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
		if len(rules) > 0 && f.path == rules[0] {
			if _, err := fmt.Fprintln(w, "exists  ", relPath(dir, f.path), "(rules)"); err != nil {
				return err
			}
			continue
		}
		if err := writeIfMissing(dir, f.path, f.content, w, dry); err != nil {
			return err
		}
	}
	if err := addHook(settings, w, dry); err != nil {
		return err
	}
	if dry {
		if _, err := fmt.Fprintln(w, "\nThis is a dry run. sv init wrote nothing."); err != nil {
			return err
		}
	} else if err := os.MkdirAll(cfg.Abs(cfg.SpecsDir), 0o755); err != nil {
		return err
	}
	hint, err := claude.MemoryHint(dir)
	if err != nil || hint == "" {
		return err
	}
	_, err = fmt.Fprintf(w, "\nAdd this line to CLAUDE.md, so that Claude uses the spec skill for each change:\n  %s\n", hint)
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
// not exist. It writes one line about the file to w. If dry is true, it
// writes no file.
func writeIfMissing(dir, path, content string, w io.Writer, dry bool) error {
	full := path
	if !filepath.IsAbs(path) {
		full = filepath.Join(dir, path)
	}
	var created bool
	var err error
	if dry {
		_, err = os.Lstat(full)
		if created = errors.Is(err, fs.ErrNotExist); created {
			err = nil
		}
	} else {
		created, err = createFile(full, content)
	}
	switch {
	case err != nil:
		return err
	case created && dry:
		_, err = fmt.Fprintln(w, "create  ", path)
	case created:
		_, err = fmt.Fprintln(w, "created ", path)
	default:
		_, err = fmt.Fprintln(w, "exists  ", path)
	}
	return err
}

// addHook adds the hook to the settings file, and writes one line about it
// to w. If dry is true, it does not change the file.
func addHook(settings string, w io.Writer, dry bool) error {
	var added bool
	var err error
	if dry {
		added, err = claude.NeedsHook(settings)
	} else {
		added, err = claude.AddHook(settings)
	}
	switch {
	case err != nil:
		return err
	case added && dry:
		_, err = fmt.Fprintln(w, "hook    ", claude.SettingsPath)
	case added:
		_, err = fmt.Fprintln(w, "hooked  ", claude.SettingsPath)
	default:
		_, err = fmt.Fprintln(w, "exists  ", claude.SettingsPath, "(sv hook)")
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

// IDRule tells which IDs are valid, in the words of the errors.
// baseline.IDPattern, from the schema, is the rule.
const IDRule = "the id starts with a letter or a digit, and contains only letters, digits, '.', '_', and '-'"

// ValidID reports whether id can be the ID of a spec.
func ValidID(id string) bool {
	return baseline.IDPattern().MatchString(id)
}

// Spec writes a new spec, made from the template, to <specs dir>/<id>.yaml
// in the project that contains dir. It returns the path of the spec,
// relative to dir. It does not overwrite a file.
func Spec(dir, id, kind, title string) (string, error) {
	cfg, err := config.Find(dir)
	if err != nil {
		return "", err
	}
	path, err := writeSpec(cfg, id, kind, title)
	if errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("%s already exists; edit it, or use another id", relPath(dir, path))
	} else if err != nil {
		return "", err
	}
	return relPath(dir, path), nil
}

// writeSpec writes a new spec to the specs directory of cfg, and returns
// its absolute path. If the file exists, it returns the path and
// fs.ErrExist.
func writeSpec(cfg *config.Config, id, kind, title string) (string, error) {
	// A valid ID cannot contain a path separator, so the file is in the specs
	// directory.
	if !ValidID(id) {
		return "", fmt.Errorf("the id %q is not valid; %s", id, IDRule)
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
		return path, fs.ErrExist
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
