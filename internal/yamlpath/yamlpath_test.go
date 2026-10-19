package yamlpath

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

const doc = `speclint: 1
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
		{"speclint", 1},
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
		{"speclint.x", 1},
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

// A key that is not a string has the text that speclint gives it when it
// converts the key: 0x1 is "1", and true is "true".
func TestLineWithKeysThatAreNotStrings(t *testing.T) {
	n := parse(t, "services:\n  0x1: manager\n  true: engine\n  name: x\n")
	for path, want := range map[string]int{"services.1": 2, "services.true": 3, "services.name": 4} {
		if got := Line(n, path); got != want {
			t.Errorf("Line(%q) = %d, want %d", path, got, want)
		}
	}
}

func TestQuotedKeys(t *testing.T) {
	src := `services:
  Order.Accessor:
    calls: [A, B]
  "a[0]":
    layer: x
  'say "hi"':
    layer: y
  OrderManager:
    calls: [C]
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]int{
		`services["Order.Accessor"]`:          2,
		`services["Order.Accessor"].calls[1]`: 3,
		`services["a[0]"].layer`:              5,
		`services["say \"hi\""].layer`:        7,
		`services.OrderManager.calls[0]`:      9,
		// Not a valid quoted key: the path stops at services.
		`services["Order.Accessor`: 1,
	} {
		if got := Line(&doc, path); got != want {
			t.Errorf("Line(%s) = %d, want %d", path, got, want)
		}
	}
}

func TestKeyGivesAPathThatLineReads(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("top:\n  first: 1\n  \"a.b [c] \\\"d\\\"\": 2\n  x-y_1: 3\n"), &doc); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]struct {
		path string
		line int
	}{
		"first":       {"top.first", 2},
		`a.b [c] "d"`: {`top["a.b [c] \"d\""]`, 3},
		"x-y_1":       {"top.x-y_1", 4},
	} {
		path := Key("top", key)
		if path != want.path || Line(&doc, path) != want.line {
			t.Errorf("Key(top, %q) = %q (line %d), want %q (line %d)", key, path, Line(&doc, path), want.path, want.line)
		}
	}
	if got := Key("", "a.b"); got != `["a.b"]` {
		t.Errorf(`Key("", "a.b") = %q`, got)
	}
}

func TestTop(t *testing.T) {
	for path, want := range map[string]string{
		`services["Order.Accessor"].calls`: "services",
		"services.OrderManager":            "services",
		"services[0]":                      "services",
		`["a.b"].c`:                        "a.b",
		"limits":                           "limits",
		"[0]":                              "",
		"":                                 "",
	} {
		if got := Top(path); got != want {
			t.Errorf("Top(%q) = %q, want %q", path, got, want)
		}
	}
}
