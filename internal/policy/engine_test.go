package policy

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tarrball/specval/internal/baseline"
	"github.com/tarrball/specval/internal/config"
)

const registry = `services:
  OrderManager: { layer: manager }
  PricingEngine: { layer: engine }
  OrderAccessor: { layer: accessor }
`

// project writes a .specval.yaml and the given files to a new directory,
// and returns the configuration. The map key is a path relative to the
// project root, and the value is the content.
func project(t *testing.T, files map[string]string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	files[config.FileName] = ""
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func newEngine(t *testing.T, cfg *config.Config) *Engine {
	t.Helper()
	eng, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return eng
}

// check returns the findings for the spec at specs/<name> in cfg.
func check(t *testing.T, cfg *config.Config, name string) []Finding {
	t.Helper()
	found, err := newEngine(t, cfg).CheckSpec(context.Background(), filepath.Join(cfg.Root, "specs", name))
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func rules(found []Finding) []string {
	ids := make([]string, 0, len(found))
	for _, f := range found {
		ids = append(ids, f.Rule)
	}
	return ids
}

func TestExampleSpecs(t *testing.T) {
	cfg, err := config.Find(filepath.Join("..", "..", "examples", "refund"))
	if err != nil {
		t.Fatal(err)
	}
	eng := newEngine(t, cfg)
	for spec, want := range map[string][]string{
		"ORD-142.yaml": nil,
		"ORD-143.yaml": {"ORD001", "SV101", "SV102", "SV107", "SV108", "SV109"},
		"ORD-144.yaml": slices.Repeat([]string{"SV001"}, 6),
	} {
		found, err := eng.CheckSpec(context.Background(), filepath.Join(cfg.Root, "specs", spec))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range found {
			if f.Fix == "" || f.Line == 0 {
				t.Errorf("%s %s: each finding needs a fix and a line: %+v", spec, f.Rule, f)
			}
		}
		got := rules(found)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", spec, got, want)
		}
	}
}

// The format that `specval context` shows to an agent must itself be a
// valid spec.
func TestFormatExampleIsAValidSpec(t *testing.T) {
	cfg := project(t, map[string]string{
		".specval/data/services.yaml": registry,
		"specs/format.yaml":           baseline.Format(),
	})
	if found := check(t, cfg, "format.yaml"); len(found) != 0 {
		t.Errorf("the format example has findings: %+v", found)
	}
}

func TestCheckSpecReportsYAMLSyntaxErrors(t *testing.T) {
	cfg := project(t, map[string]string{
		"specs/syntax.yaml":    "specval: 1\nid: a: b\n",
		"specs/duplicate.yaml": "specval: 1\nid: A\nid: B\n",
	})
	for name, line := range map[string]int{"syntax.yaml": 2, "duplicate.yaml": 3} {
		found := check(t, cfg, name)
		if len(found) != 1 || found[0].Rule != "SV000" || found[0].Severity != SeverityError || found[0].Line != line {
			t.Errorf("%s: got %+v, want one SV000 error at line %d", name, found, line)
		}
	}
}

func TestSchemaErrorsHideTheOtherRules(t *testing.T) {
	// The spec has an unknown field, and also a call to a service that is
	// not registered (SV101). CheckSpec reports only the schema error.
	cfg := project(t, map[string]string{
		".specval/data/services.yaml": registry,
		"specs/a.yaml": `specval: 1
id: A-1
kind: feature
title: T
summary: S
acceptance: [{ id: AC1, text: T }]
calls: [{ from: Nobody, to: OrderAccessor, op: Get, covers: [AC1] }]
extra: true
`,
	})
	if got := rules(check(t, cfg, "a.yaml")); !slices.Equal(got, []string{"SV001"}) {
		t.Errorf("got %v, want [SV001]", got)
	}
}

func TestCheckSpecSortsFindingsAndSetsSeverity(t *testing.T) {
	cfg := project(t, map[string]string{
		".specval/data/services.yaml": registry,
		"specs/a.yaml": `specval: 1
id: A-1
kind: feature
title: T
summary: S
acceptance:
  - { id: AC1, text: T }
  - { id: AC2, text: T }
services:
  changed: [PricingEngine]
calls:
  - { from: OrderAccessor, to: OrderManager, op: Notify, covers: [AC1] }
`,
	})
	found := check(t, cfg, "a.yaml")
	type key struct {
		rule string
		sev  Severity
		line int
	}
	var got []key
	for _, f := range found {
		got = append(got, key{f.Rule, f.Severity, f.Line})
	}
	want := []key{
		{"SV107", SeverityError, 8},
		{"SV201", SeverityWarning, 10},
		{"SV102", SeverityError, 12},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCheckSpecMissingFile(t *testing.T) {
	cfg := project(t, map[string]string{})
	if _, err := newEngine(t, cfg).CheckSpec(context.Background(), filepath.Join(cfg.Root, "nope.yaml")); !os.IsNotExist(err) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}

func TestCheckRegistryReportsAgainstTheDataFile(t *testing.T) {
	cfg := project(t, map[string]string{
		".specval/data/services.yaml": "services:\n  OrderManager: { layer: manager }\n  PricingThing: { layer: engine }\n",
		".specval/policy/registry.rego": `package specval.registry.project

import data.specval.lib
import rego.v1

# METADATA
# title: Path outside the data
# custom:
#   id: PRJ900
warn contains lib.finding(rego.metadata.rule(), "nowhere.x", "msg", "fix") if true
`,
	})
	got, err := newEngine(t, cfg).CheckRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	services := filepath.Join(cfg.Root, ".specval", "data", "services.yaml")
	if f := got[services]; len(f) != 1 || f[0].Rule != "SR002" || f[0].Line != 3 || f[0].Severity != SeverityError {
		t.Errorf("findings for services.yaml = %+v, want one SR002 error at line 3", f)
	}
	// A path whose first part is not a data key has no file and no line.
	if f := got["(data)"]; len(f) != 1 || f[0].Rule != "PRJ900" || f[0].Line != 0 || f[0].Severity != SeverityWarning {
		t.Errorf("findings for (data) = %+v, want one PRJ900 warning at line 0", f)
	}
}

func TestNewErrors(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			"a data key in two files",
			map[string]string{".specval/data/a.yaml": "services: {}", ".specval/data/b.json": `{"services": {}}`},
			`data key "services" is defined in both`,
		},
		{
			"a data file that is not a mapping",
			map[string]string{".specval/data/a.yaml": "- a\n"},
			"data files must be a mapping",
		},
		{
			"a data file that is not valid YAML",
			map[string]string{".specval/data/a.yaml": "a: [unclosed\n"},
			"a.yaml: yaml:",
		},
		{
			"a Rego syntax error",
			map[string]string{".specval/policy/a.rego": "package specval.spec.a\n\ndeny contains f if {\n"},
			"rego_parse_error",
		},
		{
			"a warn rule without custom.id",
			map[string]string{".specval/policy/a.rego": "package specval.registry.a\n\nwarn contains \"x\" if true\n"},
			"warn rule needs a METADATA block with custom.id",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := New(context.Background(), project(t, c.files))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestRulesOutsideTheSpecvalPackagesNeedNoID(t *testing.T) {
	cfg := project(t, map[string]string{
		".specval/policy/helpers.rego": "package helpers\n\ndeny contains \"x\" if true\n",
	})
	if _, err := New(context.Background(), cfg); err != nil {
		t.Errorf("New = %v, want no error", err)
	}
}

func TestAMalformedFindingIsAnError(t *testing.T) {
	cfg := project(t, map[string]string{
		".specval/policy/a.rego": `package specval.spec.a

import rego.v1

# METADATA
# title: Returns a number
# custom:
#   id: BAD001
deny contains 42 if true
`,
		"specs/a.yaml": "specval: 1\nid: A\nkind: chore\ntitle: T\nsummary: S\nacceptance: [{ id: AC1, text: T }]\ncalls: []\n",
	})
	_, err := newEngine(t, cfg).CheckSpec(context.Background(), filepath.Join(cfg.Root, "specs", "a.yaml"))
	if err == nil || !strings.Contains(err.Error(), "malformed finding") {
		t.Errorf("err = %v, want a malformed finding error", err)
	}
}

func TestRuleDocs(t *testing.T) {
	cfg := project(t, map[string]string{
		".specval/policy/a.rego": `package specval.spec.a

import rego.v1

# METADATA
# title: A project warning
# description: It explains the rule.
# custom:
#   id: PRJ201
warn contains "x" if false
`,
	})
	docs := newEngine(t, cfg).RuleDocs()
	var ids []string
	for _, d := range docs {
		ids = append(ids, d.ID)
	}
	want := []string{"PRJ201", "SV001", "SV101", "SV102", "SV103", "SV104", "SV105", "SV106", "SV107", "SV108", "SV109", "SV201"}
	if !slices.Equal(ids, want) {
		t.Errorf("IDs = %v, want %v (sorted, spec rules only)", ids, want)
	}
	want0 := RuleDoc{ID: "PRJ201", Title: "A project warning", Description: "It explains the rule.", Package: "data.specval.spec.a", Severity: SeverityWarning}
	if docs[0] != want0 {
		t.Errorf("docs[0] = %+v, want %+v", docs[0], want0)
	}
}

func TestArchitectureServicesAndDataFile(t *testing.T) {
	cfg := project(t, map[string]string{".specval/data/services.yaml": registry})
	eng := newEngine(t, cfg)

	arch, err := eng.Architecture()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(arch.LayerOrder, []string{"manager", "engine", "accessor", "resource"}) ||
		!slices.Equal(arch.Layers["manager"], []string{"engine", "accessor"}) || arch.Suffix["engine"] != "Engine" {
		t.Errorf("Architecture = %+v", arch)
	}

	services, err := eng.Services()
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 3 || services["PricingEngine"].Layer != "engine" {
		t.Errorf("Services = %+v", services)
	}

	if got, want := eng.DataFile("services"), filepath.Join(cfg.Root, ".specval", "data", "services.yaml"); got != want {
		t.Errorf("DataFile(services) = %q, want %q", got, want)
	}
	if got := eng.DataFile("nope"); got != "" {
		t.Errorf("DataFile(nope) = %q, want \"\"", got)
	}
}

func TestServicesThatIsNotAMapping(t *testing.T) {
	cfg := project(t, map[string]string{".specval/data/services.yaml": "services: [OrderManager]\n"})
	_, err := newEngine(t, cfg).Services()
	if err == nil || err.Error() != "data.project.services must be a mapping of service names to layers" {
		t.Errorf("err = %v", err)
	}
}

func TestServicesWithNoRegistry(t *testing.T) {
	services, err := newEngine(t, project(t, map[string]string{})).Services()
	if err != nil || len(services) != 0 {
		t.Errorf("Services = %v, %v; want none and no error", services, err)
	}
}

func TestAnEmptySpecReportsLineOne(t *testing.T) {
	cfg := project(t, map[string]string{"specs/empty.yaml": ""})
	found := check(t, cfg, "empty.yaml")
	if len(found) == 0 {
		t.Fatal("no findings for an empty spec")
	}
	for _, f := range found {
		if f.Line != 1 {
			t.Errorf("%s %s: line %d, want 1", f.Rule, f.Msg, f.Line)
		}
	}
}

func TestRegoErrorsStartWithTheFileAndLine(t *testing.T) {
	for name, c := range map[string]struct {
		src   string
		lines int
	}{
		"parse":   {"package specval.spec.a\n\ndeny contains f if {\n", 1},
		"compile": {"package specval.spec.a\n\nimport rego.v1\n\nx if y\n", 1},
		"two":     {"package specval.spec.a\n\nimport rego.v1\n\nx if y\n\nz if w\n", 2},
	} {
		cfg := project(t, map[string]string{".specval/policy/a.rego": c.src})
		_, err := New(context.Background(), cfg)
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		file := filepath.Join(cfg.Root, ".specval", "policy", "a.rego") + ":"
		var errors int
		for line := range strings.SplitSeq(err.Error(), "\n") {
			if strings.HasPrefix(line, "\t") {
				continue // the source line and the marker under it
			}
			errors++
			if !strings.HasPrefix(line, file) {
				t.Errorf("%s: error line %q does not start with the file and line", name, line)
			}
		}
		if errors != c.lines {
			t.Errorf("%s: %d errors in %q, want %d", name, errors, err, c.lines)
		}
	}
}

func TestRuleDocsListEachRuleOnce(t *testing.T) {
	cfg := project(t, map[string]string{
		".specval/policy/a.rego": `# METADATA
# title: Project rules
# description: Rules for this project.
package specval.spec.a

import rego.v1

# METADATA
# title: Two bodies
# custom:
#   id: PRJ001
deny contains "a" if false

# METADATA
# title: Two bodies
# custom:
#   id: PRJ001
deny contains "b" if false

# METADATA
# scope: document
# title: Document title
# custom:
#   id: PRJ002

# METADATA
# title: Rule title
# custom:
#   id: PRJ002
warn contains "c" if false
`,
	})
	var got []string
	for _, d := range newEngine(t, cfg).RuleDocs() {
		if strings.HasPrefix(d.ID, "PRJ") {
			got = append(got, d.ID+" "+d.Title)
		}
	}
	if want := []string{"PRJ001 Two bodies", "PRJ002 Rule title"}; !slices.Equal(got, want) {
		t.Errorf("RuleDocs = %q, want %q", got, want)
	}
}

func TestRuleDocsMarkTheBaselineRules(t *testing.T) {
	cfg := project(t, map[string]string{
		".specval/policy/a.rego": "package specval.spec.a\n\nimport rego.v1\n\n# METADATA\n# title: T\n# custom:\n#   id: PRJ001\ndeny contains \"a\" if false\n",
	})
	for _, d := range newEngine(t, cfg).RuleDocs() {
		if want := !strings.HasPrefix(d.ID, "PRJ"); d.Baseline != want {
			t.Errorf("%s: Baseline = %v, want %v", d.ID, d.Baseline, want)
		}
	}
}
