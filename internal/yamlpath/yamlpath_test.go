package yamlpath

import (
	"testing"

	"gopkg.in/yaml.v3"
)

const doc = `specval: 1
acceptance:
  - id: AC1
    text: First.
  - { id: AC2, text: Second. }
calls:
  - from: OrderManager
    to: OrderAccessor
services:
  new: []
`

func parse(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(src), &n); err != nil {
		t.Fatal(err)
	}
	return &n
}

func TestLine(t *testing.T) {
	n := parse(t, doc)
	for _, c := range []struct {
		path string
		want int
	}{
		{"", 1},
		{"specval", 1},
		{"acceptance", 2},
		{"acceptance[0]", 3},
		{"acceptance[0].text", 4},
		{"acceptance[1]", 5},
		{"acceptance[1].text", 5},
		{"calls[0].to", 8},
		{"services.new", 10},
		// A missing part gives the line of the deepest part that exists.
		{"calls[0].op", 7},
		{"calls[5].to", 6},
		{"nope", 1},
		// A path that goes below a scalar stops at the scalar.
		{"specval.x", 1},
		{"acceptance[0].text.x", 4},
		// An index on a mapping, or a key on a sequence, stops there.
		{"services[0]", 9},
		{"calls.from", 6},
	} {
		if got := Line(n, c.path); got != c.want {
			t.Errorf("Line(%q) = %d, want %d", c.path, got, c.want)
		}
	}
}

func TestLineEdgeCases(t *testing.T) {
	if got := Line(nil, "a"); got != 0 {
		t.Errorf("Line(nil) = %d, want 0", got)
	}
	if got := Line(&yaml.Node{Kind: yaml.DocumentNode}, "a"); got != 1 {
		t.Errorf("Line(empty document) = %d, want 1", got)
	}
	// yaml.Unmarshal gives a zero node for an empty file.
	var empty yaml.Node
	if err := yaml.Unmarshal(nil, &empty); err != nil {
		t.Fatal(err)
	}
	if got := Line(&empty, "a"); got != 1 {
		t.Errorf("Line(empty file) = %d, want 1", got)
	}
	// A mapping node that is not in a document also works.
	n := parse(t, doc).Content[0]
	if got := Line(n, "calls[0].to"); got != 8 {
		t.Errorf("Line(mapping) = %d, want 8", got)
	}
}
