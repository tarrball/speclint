package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const configName = ".specval.yaml"

type Config struct {
	Root     string   `yaml:"-"` // directory containing .specval.yaml
	SpecsDir string   `yaml:"specs_dir"`
	Policy   []string `yaml:"policy"`
	Data     []string `yaml:"data"`
}

// findConfig walks up from start to the nearest .specval.yaml.
func findConfig(start string) (*Config, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	for {
		p := filepath.Join(dir, configName)
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
			return nil, fmt.Errorf("no %s found in %s or any parent (run `specval init`)", configName, start)
		}
		dir = parent
	}
}

func (c *Config) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.Root, p)
}
