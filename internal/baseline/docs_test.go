package baseline

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestDocsListEachBuiltInRule makes sure that docs/specs.md lists each
// built-in rule, so that the documentation and the rules stay the same.
func TestDocsListEachBuiltInRule(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs.md"))
	if err != nil {
		t.Fatal(err)
	}
	mods, err := Modules()
	if err != nil {
		t.Fatal(err)
	}
	// SV000 comes from Go code, not from a Rego rule.
	ids := []string{"SV000"}
	idRe := regexp.MustCompile(`(?m)^#\s+id:\s+(\S+)$`)
	for _, m := range mods {
		for _, match := range idRe.FindAllStringSubmatch(m.Source, -1) {
			ids = append(ids, match[1])
		}
	}
	if len(ids) < 10 {
		t.Fatalf("found only %d rule IDs: %v", len(ids), ids)
	}
	for _, id := range ids {
		if !strings.Contains(string(doc), "| "+id+" |") {
			t.Errorf("docs/specs.md does not list %s in a rule table", id)
		}
	}
}
