package main

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const module = "github.com/tarrball/specval/"

// TestRegistryAgreesWithTheImports compares the service registry of specval
// (.specval/data/services.yaml) with the imports of its Go packages. Each
// import between two registered packages must be a call in the registry, and
// each call in the registry between two packages must be an import. Thus the
// registry, which the specs use, shows the code as it is.
func TestRegistryAgreesWithTheImports(t *testing.T) {
	root := filepath.Join("..", "..")
	b, err := os.ReadFile(filepath.Join(root, ".specval", "data", "services.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var reg struct {
		Services map[string]struct {
			Package string   `yaml:"package"`
			Calls   []string `yaml:"calls"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(b, &reg); err != nil {
		t.Fatal(err)
	}

	// calls maps each package to the packages that its services call.
	calls := map[string]map[string]bool{}
	for _, s := range reg.Services {
		if s.Package == "" {
			continue // A resource has no package.
		}
		if calls[s.Package] == nil {
			calls[s.Package] = map[string]bool{}
		}
		for _, c := range s.Calls {
			if p := reg.Services[c].Package; p != "" && p != s.Package {
				calls[s.Package][p] = true
			}
		}
	}

	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}}{{range .Imports}} {{.}}{{end}}`, "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	imports := map[string]map[string]bool{}
	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		pkg, ok := strings.CutPrefix(fields[0], module)
		if !ok || pkg == "cmd/sv" {
			continue // The main package reads the process state. It is not a service.
		}
		imports[pkg] = map[string]bool{}
		for _, imp := range fields[1:] {
			if p, ok := strings.CutPrefix(imp, module); ok {
				imports[pkg][p] = true
			}
		}
	}

	for _, pkg := range slices.Sorted(maps.Keys(imports)) {
		if calls[pkg] == nil {
			t.Errorf("%s is not the package of a registered service. Register its service.", pkg)
			continue
		}
		for _, imp := range slices.Sorted(maps.Keys(imports[pkg])) {
			if !calls[pkg][imp] {
				t.Errorf("%s imports %s, but no service of %s calls a service of %s in the registry. Add the call to the registry, or remove the import.", pkg, imp, pkg, imp)
			}
		}
	}
	for _, pkg := range slices.Sorted(maps.Keys(calls)) {
		if imports[pkg] == nil {
			t.Errorf("the registry has the package %s, but it does not exist", pkg)
			continue
		}
		for _, p := range slices.Sorted(maps.Keys(calls[pkg])) {
			if !imports[pkg][p] {
				t.Errorf("a service of %s calls a service of %s in the registry, but %s does not import %s. Remove the call from the registry.", pkg, p, pkg, p)
			}
		}
	}
}
