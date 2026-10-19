// Package config finds and reads the speclint configuration file, .speclint.yaml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// FileName is the name of the configuration file.
const FileName = ".speclint.yaml"

// Config is the configuration of one speclint project.
type Config struct {
	// Root is the directory that contains the configuration file.
	Root string `yaml:"-"`
	// SpecsDir is the directory that contains the specs.
	SpecsDir string `yaml:"specs_dir"`
	// Policy lists the files and directories that contain the project rules.
	Policy []string `yaml:"policy"`
	// Data lists the files and directories that contain the project data.
	Data []string `yaml:"data"`
}

// Find reads the configuration file in start or in the nearest parent of start.
func Find(start string) (*Config, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	for {
		p := filepath.Join(dir, FileName)
		b, err := os.ReadFile(p)
		if err == nil {
			c, err := parse(b)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			c.Root = dir
			return c, nil
		}
		// A configuration file that exists but cannot be read must not let the
		// search continue to a parent directory, which has other rules.
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, notFoundError{start}
		}
		dir = parent
	}
}

// ErrNotFound tells that no configuration file is in the start directory or
// in a parent directory. Thus the directory is not in a speclint project.
var ErrNotFound = errors.New("no configuration file")

// notFoundError is the error of Find when there is no configuration file.
// errors.Is finds ErrNotFound in it.
type notFoundError struct{ start string }

func (e notFoundError) Error() string {
	return fmt.Sprintf("no %s in %s or in a parent directory; run `slint init`", FileName, e.start)
}

func (notFoundError) Is(target error) bool { return target == ErrNotFound }

// Default returns the configuration that a project in root has if its
// configuration file sets no values.
func Default(root string) *Config {
	return &Config{Root: root, SpecsDir: "specs", Policy: []string{".speclint/policy"}, Data: []string{".speclint/data"}}
}

// parse reads a configuration. A key that it does not know is an error, so
// that a typing error such as "spec_dir" does not go unseen.
func parse(b []byte) (*Config, error) {
	c := Default("")
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil && !errors.Is(err, io.EOF) { // An empty file gives io.EOF.
		return nil, err
	}
	return c, nil
}

// Abs returns p as an absolute path. A relative p is relative to Root.
func (c *Config) Abs(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(c.Root, p)
}

// SpecFiles returns the YAML files in the specs directory.
func (c *Config) SpecFiles() ([]string, error) {
	return c.collect([]string{c.SpecsDir}, []string{".yaml", ".yml"})
}

// DataFiles returns the YAML and JSON files of the project data.
func (c *Config) DataFiles() ([]string, error) {
	return c.collect(c.Data, []string{".yaml", ".yml", ".json"})
}

// PolicyFiles returns the Rego files of the project rules.
// It does not return Rego test files (*_test.rego).
func (c *Config) PolicyFiles() ([]string, error) {
	paths, err := c.collect(c.Policy, []string{".rego"})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range paths {
		if isPolicy(p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// Sources holds the content of the project data files and the project rule
// files, by absolute path.
type Sources struct {
	Data     map[string][]byte
	Policies map[string][]byte
}

// ReadSources reads the project data files and the project rule files.
func (c *Config) ReadSources() (*Sources, error) {
	data, err := c.DataFiles()
	if err != nil {
		return nil, err
	}
	policies, err := c.PolicyFiles()
	if err != nil {
		return nil, err
	}
	src := &Sources{}
	if src.Data, err = readAll(data); err != nil {
		return nil, err
	}
	if src.Policies, err = readAll(policies); err != nil {
		return nil, err
	}
	return src, nil
}

// ReadSpec reads the spec file at path.
func ReadSpec(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func readAll(paths []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		out[p] = b
	}
	return out, nil
}

// isPolicy reports whether path is a Rego file that is not a Rego test file.
func isPolicy(path string) bool {
	return strings.HasSuffix(path, ".rego") && !strings.HasSuffix(path, "_test.rego")
}

// collect returns the files at paths. Each path can be a file or a directory.
// In a directory, collect returns only the files that have one of the
// extensions in exts, in lexical order. A path that does not exist gives no
// files. collect returns each file one time.
func (c *Config) collect(paths, exts []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(path string) {
		// Two configured paths can give the same file, for example a
		// directory and a file in it.
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	for _, p := range paths {
		root := c.Abs(p)
		info, err := os.Stat(root)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(root)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			for _, ext := range exts {
				if strings.HasSuffix(path, ext) {
					add(path)
					break
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
