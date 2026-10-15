package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/open-policy-agent/opa/v1/util"
	"gopkg.in/yaml.v3"
)

//go:embed baseline templates
var embedded embed.FS

type Finding struct {
	Rule     string `json:"rule"`
	Title    string `json:"title,omitempty"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Msg      string `json:"msg"`
	Fix      string `json:"fix"`
}

type RuleDoc struct {
	ID, Title, Description, Severity, Package string
}

type Engine struct {
	cfg      *Config
	data     map[string]any
	dataFile map[string]string     // top-level project data key -> file that defines it
	nodes    map[string]*yaml.Node // project data file -> parsed YAML, for line numbers
	modules  []*ast.Module
	specQ    rego.PreparedEvalQuery
	regQ     rego.PreparedEvalQuery
}

// Schema errors gate the rest: if the shape is wrong, the other rules' output is noise.
const (
	specQuery = `schema := [f | some f in data.specval.spec.schema.deny];
deny := [f | some p; p != "schema"; some f in data.specval.spec[p].deny];
warn := [f | some p; some f in data.specval.spec[p].warn]`
	registryQuery = `deny := [f | some p; some f in data.specval.registry[p].deny];
warn := [f | some p; some f in data.specval.registry[p].warn]`
)

func newEngine(ctx context.Context, cfg *Config) (*Engine, error) {
	e := &Engine{cfg: cfg, dataFile: map[string]string{}, nodes: map[string]*yaml.Node{}}

	dpl, err := baselineData()
	if err != nil {
		return nil, err
	}
	project := map[string]any{}
	for _, dir := range cfg.Data {
		if err := walkFiles(cfg.abs(dir), []string{".yaml", ".yml", ".json"}, func(path string) error {
			return e.loadDataFile(path, project)
		}); err != nil {
			return nil, err
		}
	}
	var data map[string]any
	if err := roundTrip(map[string]any{"dpl": dpl, "project": project}, &data); err != nil {
		return nil, err
	}
	e.data = data

	if err := fs.WalkDir(embedded, "baseline/policy", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isPolicy(path) {
			return err
		}
		b, err := embedded.ReadFile(path)
		if err != nil {
			return err
		}
		return e.addModule("(baseline)/"+filepath.Base(path), string(b))
	}); err != nil {
		return nil, err
	}
	for _, dir := range cfg.Policy {
		if err := walkFiles(cfg.abs(dir), []string{".rego"}, func(path string) error {
			if !isPolicy(path) {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return e.addModule(path, string(b))
		}); err != nil {
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

func isPolicy(path string) bool {
	return strings.HasSuffix(path, ".rego") && !strings.HasSuffix(path, "_test.rego")
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
			if kind != "deny" && kind != "warn" {
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

func (e *Engine) loadDataFile(path string, project map[string]any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var doc map[string]any
	if err := node.Decode(&doc); err != nil {
		return fmt.Errorf("%s: data files must be a mapping: %w", path, err)
	}
	for k, v := range doc {
		if prev, ok := e.dataFile[k]; ok {
			return fmt.Errorf("data key %q is defined in both %s and %s", k, prev, path)
		}
		project[k] = v
		e.dataFile[k] = path
	}
	e.nodes[path] = &node
	return nil
}

// baselineData is data.dpl: the DPL layer rules plus the spec schema and the
// per-path key lists and descriptions the schema rule uses to write fixes.
func baselineData() (map[string]any, error) {
	var dpl map[string]any
	b, _ := embedded.ReadFile("baseline/dpl.yaml")
	if err := yaml.Unmarshal(b, &dpl); err != nil {
		return nil, err
	}
	var schema map[string]any
	b, _ = embedded.ReadFile("baseline/schema.json")
	if err := json.Unmarshal(b, &schema); err != nil {
		return nil, err
	}
	keys, hints := map[string][]string{}, map[string]string{}
	walkSchema(schema, "", keys, hints)
	dpl["schema"], dpl["schema_keys"], dpl["schema_hints"] = schema, keys, hints
	return dpl, nil
}

func walkSchema(node map[string]any, path string, keys map[string][]string, hints map[string]string) {
	join := func(seg string) string {
		if path == "" {
			return seg
		}
		return path + "." + seg
	}
	if props, ok := node["properties"].(map[string]any); ok {
		k := path
		if k == "" {
			k = "(root)"
		}
		for name, p := range props {
			keys[k] = append(keys[k], name)
			child, _ := p.(map[string]any)
			if d, ok := child["description"].(string); ok {
				hints[join(name)] = d
			}
			walkSchema(child, join(name), keys, hints)
		}
		sort.Strings(keys[k])
	}
	if items, ok := node["items"].(map[string]any); ok {
		walkSchema(items, join("*"), keys, hints)
	}
}

// CheckSpec validates one spec file.
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
	findings, err := decodeFindings(bind["schema"], "error")
	if err != nil {
		return nil, err
	}
	if len(findings) == 0 {
		deny, err := decodeFindings(bind["deny"], "error")
		if err != nil {
			return nil, err
		}
		warn, err := decodeFindings(bind["warn"], "warning")
		if err != nil {
			return nil, err
		}
		findings = slices.Concat(deny, warn)
	}
	for i := range findings {
		findings[i].Line = lineFor(&node, findings[i].Path)
	}
	sortFindings(findings)
	return findings, nil
}

// CheckRegistry runs the registry rules once, keyed by the data file each finding is in.
func (e *Engine) CheckRegistry(ctx context.Context) (map[string][]Finding, error) {
	rs, err := e.regQ.Eval(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]Finding{}
	if len(rs) == 0 {
		return out, nil
	}
	for _, b := range []struct{ name, sev string }{{"deny", "error"}, {"warn", "warning"}} {
		found, err := decodeFindings(rs[0].Bindings[b.name], b.sev)
		if err != nil {
			return nil, err
		}
		for _, f := range found {
			key, _, _ := strings.Cut(f.Path, ".")
			file, ok := e.dataFile[key]
			if !ok {
				file = "(data)"
			} else {
				f.Line = lineFor(e.nodes[file], f.Path)
			}
			out[file] = append(out[file], f)
		}
	}
	for _, found := range out {
		sortFindings(found)
	}
	return out, nil
}

// RuleDocs lists the spec rules from their METADATA annotations.
func (e *Engine) RuleDocs() []RuleDoc {
	var docs []RuleDoc
	for _, m := range e.modules {
		pkg := m.Package.Path.String()
		if !strings.HasPrefix(pkg, "data.specval.spec.") {
			continue
		}
		for _, r := range m.Rules {
			kind := r.Head.Ref().String()
			if kind != "deny" && kind != "warn" {
				continue
			}
			for _, a := range r.Annotations {
				sev := "error"
				if kind == "warn" {
					sev = "warning"
				}
				docs = append(docs, RuleDoc{ID: ruleID(r), Title: a.Title, Description: a.Description, Severity: sev, Package: pkg})
			}
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	return docs
}

func decodeFindings(v any, severity string) ([]Finding, error) {
	var found []Finding
	if v == nil {
		return nil, nil
	}
	if err := roundTrip(v, &found); err != nil {
		return nil, fmt.Errorf("rule returned a malformed finding: %w", err)
	}
	for i := range found {
		found[i].Severity = severity
	}
	return found, nil
}

func sortFindings(found []Finding) {
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Line != found[j].Line {
			return found[i].Line < found[j].Line
		}
		return found[i].Rule < found[j].Rule
	})
}

var yamlLineRe = regexp.MustCompile(`line (\d+)`)

func yamlError(err error) Finding {
	f := Finding{Rule: "SV000", Title: "Spec is valid YAML", Severity: "error", Msg: err.Error(), Fix: "Fix the YAML syntax at this line."}
	if m := yamlLineRe.FindStringSubmatch(err.Error()); m != nil {
		f.Line, _ = strconv.Atoi(m[1])
	}
	return f
}

var indexRe = regexp.MustCompile(`\[(\d+)\]`)

// lineFor resolves a path like "calls[2].to" to its line in the YAML, falling
// back to the deepest part of the path that exists.
func lineFor(doc *yaml.Node, path string) int {
	n := doc
	if n == nil {
		return 0
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return 1
		}
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

// roundTrip converts YAML-decoded values into the JSON-compatible types OPA expects.
func roundTrip(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return util.UnmarshalJSON(b, out)
}

// walkFiles calls fn for each file under root with one of exts. A missing root is fine.
func walkFiles(root string, exts []string, fn func(string) error) error {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if !info.IsDir() {
		return fn(root)
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		for _, ext := range exts {
			if strings.HasSuffix(path, ext) {
				return fn(path)
			}
		}
		return nil
	})
}
