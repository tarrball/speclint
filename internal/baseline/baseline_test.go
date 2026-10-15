package baseline

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestSchemaIsJSON(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal([]byte(Schema()), &s); err != nil {
		t.Fatal(err)
	}
	if s["title"] != "specval spec" {
		t.Errorf("title = %v", s["title"])
	}
}

func TestKindsAndIDPatternComeFromTheSchema(t *testing.T) {
	if got, want := Kinds(), []string{"feature", "bugfix", "chore"}; !slices.Equal(got, want) {
		t.Errorf("Kinds = %v, want %v", got, want)
	}
	if got, want := IDPattern().String(), "^[A-Za-z0-9][A-Za-z0-9._-]*$"; got != want {
		t.Errorf("IDPattern = %s, want %s", got, want)
	}
}

func TestFormatIsYAML(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(Format()), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["specval"] != 1 {
		t.Errorf("specval = %v, want 1", doc["specval"])
	}
}

func TestModules(t *testing.T) {
	mods, err := Modules()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range mods {
		if m.Source == "" {
			t.Errorf("%s is empty", m.Name)
		}
		names = append(names, m.Name)
	}
	want := []string{"(baseline)/dpl.rego", "(baseline)/lib.rego", "(baseline)/registry.rego", "(baseline)/schema.rego"}
	if !slices.Equal(names, want) {
		t.Errorf("Modules = %v, want %v", names, want)
	}
}

func TestData(t *testing.T) {
	d, err := Data()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"layer_order", "layers", "suffix", "schema", "schema_keys", "schema_hints"} {
		if _, ok := d[key]; !ok {
			t.Errorf("Data has no %q", key)
		}
	}
	keys := d["schema_keys"].(map[string][]string)
	for path, want := range map[string][]string{
		"(root)":         {"acceptance", "calls", "id", "kind", "out_of_scope", "services", "specval", "summary", "title"},
		"services":       {"changed", "new"},
		"services.new.*": {"layer", "name", "purpose"},
		"calls.*":        {"covers", "from", "op", "to"},
	} {
		if got := keys[path]; !slices.Equal(got, want) {
			t.Errorf("schema_keys[%q] = %v, want %v", path, got, want)
		}
	}
	hints := d["schema_hints"].(map[string]string)
	for _, path := range []string{"id", "acceptance.*.id", "services.new.*.name", "calls.*.covers"} {
		if hints[path] == "" {
			t.Errorf("schema_hints has no hint for %q", path)
		}
	}
	// A hint comes from the description in the schema.
	if !strings.HasPrefix(hints["calls.*.op"], "PascalCase operation name") {
		t.Errorf("schema_hints[calls.*.op] = %q", hints["calls.*.op"])
	}
}
