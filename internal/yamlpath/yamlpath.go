// Package yamlpath finds the line of a path, such as "calls[2].to", in a
// YAML document.
package yamlpath

import (
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var indexRe = regexp.MustCompile(`\[(\d+)\]`)

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
	for seg := range strings.SplitSeq(indexRe.ReplaceAllString(path, ".$1"), ".") {
		if seg == "" {
			continue
		}
		var next *yaml.Node
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == seg {
					line, next = n.Content[i].Line, n.Content[i+1]
					break
				}
			}
		case yaml.SequenceNode:
			if i, err := strconv.Atoi(seg); err == nil && i < len(n.Content) {
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
