// Package policy loads the baseline and project rules and data, and
// evaluates specs and the service registry against them.
package policy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"go.yaml.in/yaml/v3"

	"github.com/tarrball/specval/internal/baseline"
	"github.com/tarrball/specval/internal/yamlpath"
)

// The queries collect the deny and warn results of each rule package. The
// spec query keeps the schema errors apart. If the schema finds an error,
// CheckSpec does not report the other findings, because the other rules
// expect the correct shape. The schema query runs only the schema rule, for
// a done spec.
const (
	schemaQuery = `schema := [f | some f in data.specval.spec.schema.deny]`
	specQuery   = schemaQuery + `;
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
	schemaQ  rego.PreparedEvalQuery
	specQ    rego.PreparedEvalQuery
	regQ     rego.PreparedEvalQuery
}

// New loads the baseline, the project data, and the project rules, and
// compiles the rules. data and policies map the path of each project data
// file and rule file to its content. The engine reads no files. It uses the
// paths only in messages, and it loads the files in the order of their paths.
func New(ctx context.Context, data, policies map[string][]byte) (*Engine, error) {
	e := &Engine{dataFile: map[string]string{}, nodes: map[string]*yaml.Node{}, baseline: map[*ast.Module]bool{}}

	dpl, err := baseline.Data()
	if err != nil {
		return nil, err
	}
	project := map[string]any{}
	for _, path := range slices.Sorted(maps.Keys(data)) {
		if err := e.loadDataFile(path, data[path], project); err != nil {
			return nil, err
		}
	}
	var all map[string]any
	if err := roundTrip(map[string]any{"dpl": dpl, "project": project}, &all); err != nil {
		return nil, err
	}
	e.data = all

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
	for _, path := range slices.Sorted(maps.Keys(policies)) {
		mod, err := e.addModule(path, string(policies[path]))
		if err != nil {
			return nil, newRegoError(err)
		}
		if err := checkSchemaCalls(mod); err != nil {
			return nil, err
		}
	}

	store := inmem.NewFromObject(all)
	if e.schemaQ, err = e.prepare(ctx, schemaQuery, store); err != nil {
		return nil, newRegoError(err)
	}
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

// capabilities gives the built-in functions that a rule can use. A rule must
// give the same findings each time for the same spec and data, and it must not
// use the network. Thus specval removes each built-in function that OPA marks
// as nondeterministic, for example http.send and time.now_ns. The schema
// functions are the exception, because SV001 uses json.match_schema.
var capabilities = sync.OnceValue(func() *ast.Capabilities {
	caps := ast.CapabilitiesForThisVersion()
	caps.Builtins = slices.DeleteFunc(caps.Builtins, func(b *ast.Builtin) bool {
		switch b.Name {
		case ast.JSONMatchSchema.Name, ast.JSONSchemaVerify.Name:
			return false
		default:
			return b.Nondeterministic
		}
	})
	return caps
})

// checkSchemaCalls returns an error if a project module uses a schema
// function. capabilities keeps these functions for SV001, but a schema can
// load a remote $ref. Thus a project rule could use the network with them.
func checkSchemaCalls(m *ast.Module) error {
	var err error
	ast.WalkTerms(m, func(t *ast.Term) bool {
		ref, ok := t.Value.(ast.Ref)
		if !ok || err != nil {
			return err != nil
		}
		for _, b := range []*ast.Builtin{ast.JSONMatchSchema, ast.JSONSchemaVerify} {
			if ref.Equal(b.Ref()) {
				err = fmt.Errorf("%s: a project rule cannot use %s, because a schema can load a remote $ref", t.Location, b.Name)
			}
		}
		return err != nil
	})
	return err
}

func (e *Engine) prepare(ctx context.Context, query string, store storage.Store) (rego.PreparedEvalQuery, error) {
	opts := []func(*rego.Rego){rego.Query(query), rego.Store(store), rego.Capabilities(capabilities())}
	for _, m := range e.modules {
		opts = append(opts, rego.ParsedModule(m))
	}
	return rego.New(opts...).PrepareForEval(ctx)
}

// SpecResult is the result of the check of one spec.
type SpecResult struct {
	Findings []Finding
	// Stopped is true if the spec has format errors (SV000 or SV001), and
	// thus the other rules did not run. They run when the format is correct.
	// It is false for a done spec, because the other rules never run on it.
	Stopped bool
}

// CheckSpec validates the content of a spec file.
func (e *Engine) CheckSpec(ctx context.Context, b []byte) (SpecResult, error) {
	var node yaml.Node
	var doc any
	// specval cannot read the status of a spec that is not valid YAML, so it
	// does not know if the spec is done.
	second, err := parseOne(b, &node)
	if err != nil {
		return stopped(false, yamlError(err)), nil
	}
	if second > 0 {
		return stopped(false, Finding{
			Rule: "SV000", Title: "Spec is valid YAML", Severity: SeverityError, Line: second,
			Msg: "The spec has more than one YAML document. specval reads only the first.",
			Fix: "Remove the line with three dashes and the document after it. Put each spec in its own file.",
		}), nil
	}
	if len(node.Content) > 0 {
		if err := node.Decode(&doc); err != nil {
			return stopped(false, yamlError(err)), nil
		}
	}
	if doc == nil {
		doc = map[string]any{}
	}
	done := isDone(doc)
	doc, err = stringKeys(doc, "")
	if keyErr, ok := errors.AsType[keyError](err); ok {
		return stopped(done, Finding{
			Rule: "SV000", Title: "Spec is valid YAML", Severity: SeverityError,
			Path: keyErr.path, Line: yamlpath.Line(&node, keyErr.path),
			Msg: err.Error(), Fix: "Give each key in this mapping a different name.",
		}), nil
	}
	var input any
	if err := roundTrip(doc, &input); err != nil {
		return stopped(done, yamlError(err)), nil
	}
	query := e.specQ
	if done {
		// The other rules do not run. Thus an error in one of them cannot
		// stop the check of a done spec.
		query = e.schemaQ
	}
	rs, err := eval(ctx, query, rego.EvalInput(input))
	if err != nil {
		return SpecResult{}, err
	}
	if len(rs) == 0 {
		return SpecResult{}, errors.New("policy query returned no result")
	}
	bind := rs[0].Bindings
	findings, err := decodeFindings(bind["schema"], SeverityError)
	if err != nil {
		return SpecResult{}, err
	}
	// The other rules expect the correct shape, so they run only if the
	// schema rule finds no error.
	res := SpecResult{Stopped: len(findings) > 0 && !done}
	if len(findings) == 0 {
		deny, err := decodeFindings(bind["deny"], SeverityError)
		if err != nil {
			return SpecResult{}, err
		}
		warn, err := decodeFindings(bind["warn"], SeverityWarning)
		if err != nil {
			return SpecResult{}, err
		}
		findings = slices.Concat(deny, warn)
	}
	for i := range findings {
		findings[i].Line = yamlpath.Line(&node, findings[i].Path)
	}
	sortFindings(findings)
	res.Findings = findings
	return res, nil
}

// stopped returns the result for a spec that is not valid YAML. The rules
// cannot run on it. If the spec is done, the other rules never run, so
// Stopped is false.
func stopped(done bool, found ...Finding) SpecResult {
	return SpecResult{Findings: found, Stopped: !done}
}

// statusDone is the status of a spec whose change is implemented.
const statusDone = "done"

// isDone reports whether the spec has the status done. specval runs only
// the schema rule on such a spec. The other rules use the registry as it is now,
// and the change of the spec is already in the registry. For example, SV104
// would report each service in services.new.
func isDone(spec any) bool {
	m, _ := spec.(map[string]any)
	return m["status"] == statusDone
}

// evalTimeout is the time limit for one evaluation of the rules: the rules
// for one spec, or the registry rules. A rule with a very large loop must not
// run until Claude Code stops the hook.
var evalTimeout = 10 * time.Second

// eval evaluates q with the time limit. If the limit stops the evaluation,
// the error tells that a rule did not finish.
func eval(ctx context.Context, q rego.PreparedEvalQuery, opts ...rego.EvalOption) (rego.ResultSet, error) {
	ctx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	rs, err := q.Eval(ctx, opts...)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("the rules did not finish in %s; a rule can have a loop that is too large", evalTimeout)
	}
	return rs, err
}

// CheckRegistry runs the registry rules. It returns the findings for each
// data file.
func (e *Engine) CheckRegistry(ctx context.Context) (map[string][]Finding, error) {
	rs, err := eval(ctx, e.regQ)
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
			file, ok := e.dataFile[yamlpath.Top(f.Path)]
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
// annotations. It lists a rule ID that has more than one rule body one time.
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
