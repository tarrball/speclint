// Package policy loads the baseline and project rules and data, and
// evaluates specs and the service registry against them.
package policy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"gopkg.in/yaml.v3"

	"github.com/tarrball/specval/internal/baseline"
	"github.com/tarrball/specval/internal/config"
	"github.com/tarrball/specval/internal/yamlpath"
)

// The queries collect the deny and warn results of each rule package.
// Schema errors gate the rest: if the shape is wrong, the output of the other
// rules is noise.
const (
	specQuery = `schema := [f | some f in data.specval.spec.schema.deny];
deny := [f | some p; p != "schema"; some f in data.specval.spec[p].deny];
warn := [f | some p; some f in data.specval.spec[p].warn]`
	registryQuery = `deny := [f | some p; some f in data.specval.registry[p].deny];
warn := [f | some p; some f in data.specval.registry[p].warn]`
)

// Engine evaluates specs and the service registry for one project.
type Engine struct {
	data     map[string]any
	dataFile map[string]string     // top-level project data key -> file that defines it
	nodes    map[string]*yaml.Node // project data file -> parsed YAML, for line numbers
	modules  []*ast.Module
	specQ    rego.PreparedEvalQuery
	regQ     rego.PreparedEvalQuery
}

// New loads the baseline and the project data and rules of cfg, and
// compiles the rules.
func New(ctx context.Context, cfg *config.Config) (*Engine, error) {
	e := &Engine{dataFile: map[string]string{}, nodes: map[string]*yaml.Node{}}

	dpl, err := baseline.Data()
	if err != nil {
		return nil, err
	}
	dataFiles, err := cfg.DataFiles()
	if err != nil {
		return nil, err
	}
	project := map[string]any{}
	for _, path := range dataFiles {
		if err := e.loadDataFile(path, project); err != nil {
			return nil, err
		}
	}
	var data map[string]any
	if err := roundTrip(map[string]any{"dpl": dpl, "project": project}, &data); err != nil {
		return nil, err
	}
	e.data = data

	mods, err := baseline.Modules()
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		if err := e.addModule(m.Name, m.Source); err != nil {
			return nil, err
		}
	}
	policyFiles, err := cfg.PolicyFiles()
	if err != nil {
		return nil, err
	}
	for _, path := range policyFiles {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := e.addModule(path, string(b)); err != nil {
			return nil, err
		}
	}

	store := inmem.NewFromObject(data)
	if e.specQ, err = e.prepare(ctx, specQuery, store); err != nil {
		return nil, err
	}
	if e.regQ, err = e.prepare(ctx, registryQuery, store); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Engine) addModule(name, src string) error {
	m, err := ast.ParseModuleWithOpts(name, src, ast.ParserOptions{ProcessAnnotation: true, RegoVersion: ast.RegoV1})
	if err != nil {
		return err
	}
	pkg := m.Package.Path.String()
	if strings.HasPrefix(pkg, "data.specval.spec.") || strings.HasPrefix(pkg, "data.specval.registry.") {
		for _, r := range m.Rules {
			kind := r.Head.Ref().String()
			if _, ok := ruleSeverity[kind]; !ok {
				continue
			}
			if ruleID(r) == "" {
				return fmt.Errorf("%s: %s rule needs a METADATA block with custom.id", r.Location, kind)
			}
		}
	}
	e.modules = append(e.modules, m)
	return nil
}

func ruleID(r *ast.Rule) string {
	for _, a := range r.Annotations {
		if id, ok := a.Custom["id"].(string); ok {
			return id
		}
	}
	return ""
}

func (e *Engine) prepare(ctx context.Context, query string, store storage.Store) (rego.PreparedEvalQuery, error) {
	opts := []func(*rego.Rego){rego.Query(query), rego.Store(store)}
	for _, m := range e.modules {
		opts = append(opts, rego.ParsedModule(m))
	}
	return rego.New(opts...).PrepareForEval(ctx)
}

// CheckSpec validates the spec file at path.
func (e *Engine) CheckSpec(ctx context.Context, path string) ([]Finding, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	var doc any
	if err := yaml.Unmarshal(b, &node); err != nil {
		return []Finding{yamlError(err)}, nil
	}
	if len(node.Content) > 0 {
		if err := node.Decode(&doc); err != nil {
			return []Finding{yamlError(err)}, nil
		}
	}
	if doc == nil {
		doc = map[string]any{}
	}
	var input any
	if err := roundTrip(doc, &input); err != nil {
		return []Finding{yamlError(err)}, nil
	}
	rs, err := e.specQ.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, err
	}
	if len(rs) == 0 {
		return nil, errors.New("policy query returned no result")
	}
	bind := rs[0].Bindings
	findings, err := decodeFindings(bind["schema"], SeverityError)
	if err != nil {
		return nil, err
	}
	if len(findings) == 0 {
		deny, err := decodeFindings(bind["deny"], SeverityError)
		if err != nil {
			return nil, err
		}
		warn, err := decodeFindings(bind["warn"], SeverityWarning)
		if err != nil {
			return nil, err
		}
		findings = slices.Concat(deny, warn)
	}
	for i := range findings {
		findings[i].Line = yamlpath.Line(&node, findings[i].Path)
	}
	sortFindings(findings)
	return findings, nil
}

// CheckRegistry runs the registry rules. It returns the findings for each
// data file.
func (e *Engine) CheckRegistry(ctx context.Context) (map[string][]Finding, error) {
	rs, err := e.regQ.Eval(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]Finding{}
	if len(rs) == 0 {
		return out, nil
	}
	for _, name := range []string{"deny", "warn"} {
		found, err := decodeFindings(rs[0].Bindings[name], ruleSeverity[name])
		if err != nil {
			return nil, err
		}
		for _, f := range found {
			key, _, _ := strings.Cut(f.Path, ".")
			file, ok := e.dataFile[key]
			if !ok {
				file = "(data)"
			} else {
				f.Line = yamlpath.Line(e.nodes[file], f.Path)
			}
			out[file] = append(out[file], f)
		}
	}
	for _, found := range out {
		sortFindings(found)
	}
	return out, nil
}

// RuleDocs returns the spec rules, sorted by ID, from their METADATA annotations.
func (e *Engine) RuleDocs() []RuleDoc {
	var docs []RuleDoc
	for _, m := range e.modules {
		pkg := m.Package.Path.String()
		if !strings.HasPrefix(pkg, "data.specval.spec.") {
			continue
		}
		for _, r := range m.Rules {
			sev, ok := ruleSeverity[r.Head.Ref().String()]
			if !ok {
				continue
			}
			for _, a := range r.Annotations {
				docs = append(docs, RuleDoc{ID: ruleID(r), Title: a.Title, Description: a.Description, Severity: sev, Package: pkg})
			}
		}
	}
	slices.SortStableFunc(docs, func(a, b RuleDoc) int { return cmp.Compare(a.ID, b.ID) })
	return docs
}
