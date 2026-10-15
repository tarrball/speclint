// Package baseline holds the built-in spec schema, the layer data, and the
// rules that apply to every project.
package baseline

import (
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	//go:embed schema.json
	schema string

	//go:embed format.yaml
	format string

	//go:embed dpl.yaml
	dpl string

	//go:embed policy/*.rego
	policies embed.FS
)

// Schema returns the JSON Schema of a spec.
func Schema() string { return schema }

// Format returns an annotated example spec. It shows an agent the format.
func Format() string { return format }

// Module is the source of one Rego file.
type Module struct {
	// Name identifies the module in error messages, for example "(baseline)/dpl.rego".
	Name   string
	Source string
}

// Modules returns the baseline Rego rules. It does not return Rego test files.
func Modules() ([]Module, error) {
	var mods []Module
	err := fs.WalkDir(policies, "policy", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".rego") || strings.HasSuffix(p, "_test.rego") {
			return err
		}
		b, err := policies.ReadFile(p)
		if err != nil {
			return err
		}
		mods = append(mods, Module{Name: "(baseline)/" + path.Base(p), Source: string(b)})
		return nil
	})
	return mods, err
}

// Data returns the document that the rules see as data.dpl: the layer rules
// from dpl.yaml, plus the schema and the per-path key lists and descriptions
// that the schema rule uses to write fixes.
func Data() (map[string]any, error) {
	var d map[string]any
	if err := yaml.Unmarshal([]byte(dpl), &d); err != nil {
		return nil, err
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(schema), &s); err != nil {
		return nil, err
	}
	keys, hints := map[string][]string{}, map[string]string{}
	walkSchema(s, "", keys, hints)
	d["schema"], d["schema_keys"], d["schema_hints"] = s, keys, hints
	return d, nil
}

func walkSchema(node map[string]any, prefix string, keys map[string][]string, hints map[string]string) {
	join := func(seg string) string {
		if prefix == "" {
			return seg
		}
		return prefix + "." + seg
	}
	if props, ok := node["properties"].(map[string]any); ok {
		k := prefix
		if k == "" {
			k = "(root)"
		}
		for name, p := range props {
			keys[k] = append(keys[k], name)
			child, _ := p.(map[string]any)
			if d, ok := child["description"].(string); ok {
				hints[join(name)] = d
			}
			walkSchema(child, join(name), keys, hints)
		}
		sort.Strings(keys[k])
	}
	if items, ok := node["items"].(map[string]any); ok {
		walkSchema(items, join("*"), keys, hints)
	}
}
