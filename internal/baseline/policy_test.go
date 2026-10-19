package baseline

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/open-policy-agent/opa/v1/tester"
	"github.com/open-policy-agent/opa/v1/util"
)

// TestRegoPolicies runs the Rego tests (policy/*_test.rego) with data.dpl
// from Data, as speclint gives it to the rules.
func TestRegoPolicies(t *testing.T) {
	ctx := context.Background()
	modules := map[string]*ast.Module{}
	err := fs.WalkDir(policies, "policy", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".rego") {
			return err
		}
		src, err := policies.ReadFile(p)
		if err != nil {
			return err
		}
		m, err := ast.ParseModuleWithOpts(p, string(src), ast.ParserOptions{ProcessAnnotation: true, RegoVersion: ast.RegoV1})
		if err != nil {
			return err
		}
		modules[p] = m
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	dpl, err := Data()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]any{"dpl": dpl, "project": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := util.UnmarshalJSON(b, &data); err != nil {
		t.Fatal(err)
	}

	store := inmem.NewFromObject(data)
	txn, err := store.NewTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Abort(ctx, txn)
	results, err := tester.NewRunner().
		SetStore(store).
		SetModules(modules).
		RaiseBuiltinErrors(true).
		RunTests(ctx, txn)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for r := range results {
		n++
		switch {
		case r.Error != nil:
			t.Errorf("%s.%s: %v", r.Package, r.Name, r.Error)
		case r.Fail:
			t.Errorf("%s.%s failed (%s)", r.Package, r.Name, r.Location)
		}
	}
	if n == 0 {
		t.Error("found no Rego tests")
	}
}
