// Package config finds and reads the specval configuration file, .specval.yaml.
package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// FileName is the name of the configuration file.
const FileName = ".specval.yaml"

// Config is the configuration of one specval project.
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
		if b, err := os.ReadFile(p); err == nil {
			c := &Config{SpecsDir: "specs", Policy: []string{".specval/policy"}, Data: []string{".specval/data"}}
			if err := yaml.Unmarshal(b, c); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			c.Root = dir
			return c, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("no %s found in %s or any parent (run `specval init`)", FileName, start)
		}
		dir = parent
	}
}

// Abs returns p as an absolute path. A relative p is relative to Root.
func (c *Config) Abs(p string) string {
	if filepath.IsAbs(p) {
		return p
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

// isPolicy reports whether path is a Rego file that is not a Rego test file.
func isPolicy(path string) bool {
	return strings.HasSuffix(path, ".rego") && !strings.HasSuffix(path, "_test.rego")
}

// collect returns the files at paths, in lexical order. Each path can be a
// file or a directory. In a directory, collect returns only the files that
// have one of the extensions in exts. A path that does not exist gives no files.
func (c *Config) collect(paths, exts []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		root := c.Abs(p)
		info, err := os.Stat(root)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, root)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			for _, ext := range exts {
				if strings.HasSuffix(path, ext) {
					out = append(out, path)
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
