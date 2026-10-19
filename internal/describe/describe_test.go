package describe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tarrball/speclint/internal/config"
)

func TestFind(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		".speclint.yaml":               "specs_dir: plans\n",
		".speclint/data/services.yaml": "services:\n  OrderManager: { layer: manager, may_call: [accessor] }\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Find(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.SpecsDir != "plans" {
		t.Errorf("SpecsDir = %q, want plans", p.SpecsDir)
	}
	if !slices.Contains(p.Architecture.LayerOrder, "manager") {
		t.Errorf("LayerOrder = %v", p.Architecture.LayerOrder)
	}
	if s, ok := p.Services["OrderManager"]; !ok || s.Layer != "manager" || !slices.Equal(s.MayCall, []string{"accessor"}) {
		t.Errorf("Services = %+v", p.Services)
	}
	if !slices.ContainsFunc(p.Rules, func(d RuleDoc) bool { return d.ID == "SV101" }) {
		t.Errorf("Rules has no SV101: %+v", p.Rules)
	}
}

func TestFindOutsideAProject(t *testing.T) {
	if _, err := Find(context.Background(), t.TempDir()); !errors.Is(err, config.ErrNotFound) {
		t.Errorf("err = %v, want config.ErrNotFound", err)
	}
}
