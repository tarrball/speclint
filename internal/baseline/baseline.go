// Package baseline holds the built-in spec schema, the layer data, and the
// rules that apply to every project.
package baseline

import (
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
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

// specFields holds the parts of the schema that Go code also uses. Thus the
// schema is the one source of these values.
type specFields struct {
	kinds  []string
	id     *regexp.Regexp
	format int
}

var schemaFields = sync.OnceValue(func() specFields {
	var s struct {
		Properties struct {
			Specval struct {
				Const int `json:"const"`
			} `json:"specval"`
			Kind struct {
				Enum []string `json:"enum"`
			} `json:"kind"`
			ID struct {
				Pattern string `json:"pattern"`
			} `json:"id"`
		} `json:"properties"`
	}
	// The binary contains the schema, and the tests parse it. Thus an error here is a
	// defect in the build, not in the input.
	if err := json.Unmarshal([]byte(schema), &s); err != nil {
		panic("baseline: schema.json: " + err.Error())
	}
	return specFields{
		kinds:  s.Properties.Kind.Enum,
		id:     regexp.MustCompile(s.Properties.ID.Pattern),
		format: s.Properties.Specval.Const,
	}
})

// Kinds returns the values that the kind of a spec can have.
func Kinds() []string { return slices.Clone(schemaFields().kinds) }

// FormatVersion returns the version of the spec format: the value of the
// specval field of a spec.
func FormatVersion() int { return schemaFields().format }

// IDPattern returns the pattern that the ID of a spec must match.
func IDPattern() *regexp.Regexp { return schemaFields().id }

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

// Data returns the document that the rules see as data.dpl. It contains the
// layer rules from dpl.yaml and the schema. It also contains tables of the
// schema that the schema rule uses to write fixes. Each table maps a path, for
// example "calls.*.op" ("*" is an array index), to a value:
//
//   - schema_keys: the permitted keys of the object at the path
//   - schema_hints: the description of the path
//   - schema_types: the type of the path, for example "object"
//   - schema_required: the required keys of the object at the path
func Data() (map[string]any, error) {
	var d map[string]any
	if err := yaml.Unmarshal([]byte(dpl), &d); err != nil {
		return nil, err
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(schema), &s); err != nil {
		return nil, err
	}
	t := schemaTables{keys: map[string][]string{}, hints: map[string]string{}, types: map[string]string{}, required: map[string][]string{}}
	t.walk(s, "")
	d["schema"], d["schema_keys"], d["schema_hints"] = s, t.keys, t.hints
	d["schema_types"], d["schema_required"] = t.types, t.required
	return d, nil
}

// schemaTables holds the tables that Data makes from the schema.
type schemaTables struct {
	keys     map[string][]string
	hints    map[string]string
	types    map[string]string
	required map[string][]string
}

func (t schemaTables) walk(node map[string]any, prefix string) {
	join := func(seg string) string {
		if prefix == "" {
			return seg
		}
		return prefix + "." + seg
	}
	k := prefix
	if k == "" {
		k = "(root)"
	}
	if typ, ok := node["type"].(string); ok {
		t.types[k] = typ
	}
	if req, ok := node["required"].([]any); ok {
		for _, r := range req {
			if name, ok := r.(string); ok {
				t.required[k] = append(t.required[k], name)
			}
		}
		sort.Strings(t.required[k])
	}
	if props, ok := node["properties"].(map[string]any); ok {
		for name, p := range props {
			t.keys[k] = append(t.keys[k], name)
			child, _ := p.(map[string]any)
			if d, ok := child["description"].(string); ok {
				t.hints[join(name)] = d
			}
			t.walk(child, join(name))
		}
		sort.Strings(t.keys[k])
	}
	if items, ok := node["items"].(map[string]any); ok {
		t.walk(items, join("*"))
	}
}
