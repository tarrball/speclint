// Package yamlpath finds the line of a path, such as "calls[2].to", in a
// YAML document.
//
// A path is keys and indexes. A key follows a dot, or is first. A key with a
// character other than a letter, a digit, '_', or '-' is in brackets and
// quotes, as a JSON string: services["Order.Accessor"].calls[0].
package yamlpath

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// plainKey matches a key that a path can have after a dot.
var plainKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Key returns the path of key in the mapping at prefix. An empty prefix is
// the top level. lib.path_key in the baseline Rego rules does the same.
func Key(prefix, key string) string {
	if !plainKey.MatchString(key) {
		b, _ := json.Marshal(key) // A string always encodes.
		return prefix + "[" + string(b) + "]"
	}
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// Top returns the first key of path, or "" if path does not start with a
// key. For example, the top of services["Order.Accessor"].calls is services.
func Top(path string) string {
	if segs := segments(path); len(segs) > 0 && segs[0].isKey {
		return segs[0].key
	}
	return ""
}

// segment is one part of a path: a key, or an index in a list.
type segment struct {
	key   string
	index int
	isKey bool
}

// segments reads a path into its parts. A part that it cannot read is a key
// with the rest of the path as its text, so that Line stops there.
func segments(path string) []segment {
	var out []segment
	for path != "" {
		switch {
		case path[0] == '.':
			path = path[1:]
		case strings.HasPrefix(path, `["`):
			var key string
			dec := json.NewDecoder(strings.NewReader(path[1:]))
			if err := dec.Decode(&key); err != nil {
				return append(out, segment{key: path, isKey: true})
			}
			rest := path[1+int(dec.InputOffset()):]
			if !strings.HasPrefix(rest, "]") {
				return append(out, segment{key: path, isKey: true})
			}
			out = append(out, segment{key: key, isKey: true})
			path = rest[1:]
		case path[0] == '[':
			end := strings.IndexByte(path, ']')
			i, err := strconv.Atoi(path[1:max(end, 1)])
			if end < 0 || err != nil {
				return append(out, segment{key: path, isKey: true})
			}
			out = append(out, segment{index: i})
			path = path[end+1:]
		default:
			end := strings.IndexAny(path, ".[")
			if end < 0 {
				end = len(path)
			}
			out = append(out, segment{key: path[:end], isKey: true})
			path = path[end:]
		}
	}
	return out
}

// text returns the segment as the text of a mapping key. An index i is the
// key "i", as when specval converts YAML to JSON.
func (s segment) text() string {
	if s.isKey {
		return s.key
	}
	return strconv.Itoa(s.index)
}

// Line returns the line of path in doc. If a part of the path is not in the
// document, Line returns the line of the deepest part that is.
func Line(doc *yaml.Node, path string) int {
	n := doc
	if n == nil {
		return 0
	}
	// An empty file gives a zero node, or a document with no content.
	if n.Kind == 0 || (n.Kind == yaml.DocumentNode && len(n.Content) == 0) {
		return 1
	}
	if n.Kind == yaml.DocumentNode {
		n = n.Content[0]
	}
	line := n.Line
	for _, seg := range segments(path) {
		var next *yaml.Node
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if keyText(n.Content[i]) == seg.text() {
					line, next = n.Content[i].Line, n.Content[i+1]
					break
				}
			}
		case yaml.SequenceNode:
			// A key of digits is an index too, as in the paths of OPA.
			if i, err := strconv.Atoi(seg.text()); err == nil && i >= 0 && i < len(n.Content) {
				next = n.Content[i]
				line = next.Line
			}
		default:
			// Scalars and aliases have no children, so the path stops here.
		}
		if next == nil {
			break
		}
		n = next
	}
	return line
}

// keyText returns the text of a mapping key, as specval writes it when it
// converts YAML to JSON: the key 0x1 is "1". A key that is a string is its
// value.
func keyText(key *yaml.Node) string {
	var v any
	if key.Kind != yaml.ScalarNode || key.Decode(&v) != nil {
		return key.Value
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
