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
	"go.yaml.in/yaml/v3"

	"github.com/tarrball/specval/internal/baseline"
	"github.com/tarrball/specval/internal/config"
	"github.com/tarrball/specval/internal/yamlpath"
)

// The queries collect the deny and warn results of each rule package. The
// spec query keeps the schema errors apart. If the schema finds an error,
// CheckSpec does not report the other findings, because the other rules
// expect the correct shape.
const (
	specQuery = `schema := [f | some f in data.specval.spec.schema.deny];
deny := [f | some p; p != "schema"; some f in data.specval.spec[p].deny];
warn := [f | some p; some f in data.specval.spec[p].warn]`
	registryQuery = `deny := [f | some p; some f in data.specval.registry[p].deny];
warn := [f | some p; some f in data.specval.registry[p].warn]`
)

// Engine evaluates specs and the service registry for one project.
type Engine struct {
	data map[string]any
	// dataFile maps each top-level key of the project data to the file that
	// defines it.
	dataFile map[string]string
	// nodes maps each project data file to its YAML nodes, which give the
	// line numbers.
	nodes   map[string]*yaml.Node
	modules []*ast.Module
	// baseline holds the modules that are built into specval.
	baseline map[*ast.Module]bool
	specQ    rego.PreparedEvalQuery
	regQ     rego.PreparedEvalQuery
}

// New loads the baseline and the project data and rules of cfg, and
// compiles the rules.
func New(ctx context.Context, cfg *config.Config) (*Engine, error) {
	e := &Engine{dataFile: map[string]string{}, nodes: map[string]*yaml.Node{}, baseline: map[*ast.Module]bool{}}

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
		mod, err := e.addModule(m.Name, m.Source)
		if err != nil {
			return nil, newRegoError(err)
		}
		e.baseline[mod] = true
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
		if _, err := e.addModule(path, string(b)); err != nil {
			return nil, newRegoError(err)
		}
	}

	store := inmem.NewFromObject(data)
	if e.specQ, err = e.prepare(ctx, specQuery, store); err != nil {
		return nil, newRegoError(err)
	}
	if e.regQ, err = e.prepare(ctx, registryQuery, store); err != nil {
		return nil, newRegoError(err)
	}
	return e, nil
}

// regoError is a list of Rego parse or compile errors. Its text has one
// error on each line, and each error starts with its file and line.
type regoError struct{ errs ast.Errors }

func (e regoError) Error() string {
	msgs := make([]string, len(e.errs))
	for i, err := range e.errs {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "\n")
}

func (e regoError) Unwrap() error { return e.errs }

// newRegoError removes the text that OPA puts around a list of errors, such as
// "bundle activation failed: 1 error occurred: ".
func newRegoError(err error) error {
	var errs ast.Errors
	if errors.As(err, &errs) && len(errs) > 0 {
		return regoError{errs}
	}
	return err
}

func (e *Engine) addModule(name, src string) (*ast.Module, error) {
	m, err := ast.ParseModuleWithOpts(name, src, ast.ParserOptions{ProcessAnnotation: true, RegoVersion: ast.RegoV1})
	if err != nil {
		return nil, err
	}
	if kind := ruleKind(m.Package.Path); kind != "" {
		// The queries read only the packages one level under specval.spec and
		// specval.registry, so a deeper package would load but never run.
		if len(m.Package.Path) > 4 {
			return nil, fmt.Errorf("%s: package %s is too deep; use specval.%s.<name>",
				m.Package.Location, strings.TrimPrefix(m.Package.Path.String(), "data."), kind)
		}
		for _, r := range m.Rules {
			kind := r.Head.Ref().String()
			if _, ok := ruleSeverity[kind]; !ok {
				continue
			}
			if ruleAnnotation(r) == nil {
				return nil, fmt.Errorf("%s: %s rule needs a METADATA block with custom.id", r.Location, kind)
			}
		}
	}
	e.modules = append(e.modules, m)
	return m, nil
}

// ruleKind returns "spec" for a package under data.specval.spec, "registry"
// for a package under data.specval.registry, and "" for any other package.
// It reads the parts of the path, so a part in bracket notation, such as
// specval.spec["pricing-rules"], counts as one part.
func ruleKind(path ast.Ref) string {
	if len(path) < 4 || !path[1].Equal(ast.StringTerm("specval")) {
		return ""
	}
	for _, kind := range []string{"spec", "registry"} {
		if path[2].Equal(ast.StringTerm(kind)) {
			return kind
		}
	}
	return ""
}

// ruleAnnotation returns the METADATA annotation of r that has a custom.id.
// It prefers an annotation with rule scope to one with a wider scope.
func ruleAnnotation(r *ast.Rule) *ast.Annotations {
	var found *ast.Annotations
	for _, a := range r.Annotations {
		if id, ok := a.Custom["id"].(string); !ok || id == "" {
			continue
		}
		if a.Scope == "rule" {
			return a
		}
		if found == nil {
			found = a
		}
	}
	return found
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
	doc, err = stringKeys(doc, "")
	if keyErr, ok := errors.AsType[keyError](err); ok {
		return []Finding{{
			Rule: "SV000", Title: "Spec is valid YAML", Severity: SeverityError,
			Path: keyErr.path, Line: yamlpath.Line(&node, keyErr.path),
			Msg: err.Error(), Fix: "Give each key in this mapping a different name.",
		}}, nil
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

// RuleDocs returns the spec rules, sorted by ID, from their METADATA
// annotations. A rule ID that has more than one rule body is listed once.
func (e *Engine) RuleDocs() []RuleDoc {
	var docs []RuleDoc
	seen := map[string]bool{}
	for _, m := range e.modules {
		if ruleKind(m.Package.Path) != "spec" {
			continue
		}
		for _, r := range m.Rules {
			sev, ok := ruleSeverity[r.Head.Ref().String()]
			a := ruleAnnotation(r)
			if !ok || a == nil {
				continue
			}
			id := a.Custom["id"].(string)
			if seen[id] {
				continue
			}
			seen[id] = true
			docs = append(docs, RuleDoc{
				ID: id, Title: a.Title, Description: a.Description,
				Severity: sev, Baseline: e.baseline[m],
			})
		}
	}
	slices.SortStableFunc(docs, func(a, b RuleDoc) int { return cmp.Compare(a.ID, b.ID) })
	return docs
}
