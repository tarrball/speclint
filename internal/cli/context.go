package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tarrball/specval/internal/baseline"
	"github.com/tarrball/specval/internal/config"
	"github.com/tarrball/specval/internal/policy"
)

func cmdContext(ctx context.Context, env *Env, args []string) error {
	if err := parseNoArgs(newFlagSet("context", env), args); err != nil {
		return err
	}
	dir, err := env.workDir()
	if err != nil {
		return err
	}
	cfg, err := config.Find(dir)
	if err != nil {
		return err
	}
	eng, err := policy.New(ctx, cfg)
	if err != nil {
		return err
	}
	arch, err := eng.Architecture()
	if err != nil {
		return err
	}
	registry, err := eng.Services()
	if err != nil {
		return err
	}
	w := env.Stdout

	fmt.Fprintf(w, "# Spec rules (specval)\n\nEach change needs a spec at %s/<id>.yaml, and the spec must pass before you write code. To make a new spec, run `specval new <id>`. To check a spec, run `specval check <file>`. Each finding has a fix.\n\n", cfg.SpecsDir)
	fmt.Fprintf(w, "## Format\n```yaml\n%s```\n\n", baseline.Format())

	fmt.Fprint(w, "## Layers\nEach layer can call only the layers after its arrow.\n")
	var suffixes []string
	for _, l := range arch.LayerOrder {
		targets := strings.Join(arch.Layers[l], ", ")
		if targets == "" {
			targets = "nothing"
		}
		fmt.Fprintf(w, "- %s -> %s\n", l, targets)
		suffixes = append(suffixes, arch.Suffix[l])
	}
	fmt.Fprintf(w, "The name of each service ends with the name of its layer: %s.\n\n", strings.Join(suffixes, ", "))

	fmt.Fprint(w, "## Registered services\n")
	byLayer := map[string][]string{}
	for name, s := range registry {
		byLayer[s.Layer] = append(byLayer[s.Layer], name)
	}
	for _, l := range arch.LayerOrder {
		if names := byLayer[l]; len(names) > 0 {
			sort.Strings(names)
			fmt.Fprintf(w, "- %s: %s\n", l, strings.Join(names, ", "))
		}
		delete(byLayer, l)
	}
	// The registry rules report the services that are left, because their
	// layer is not valid.
	var invalid []string
	for _, names := range byLayer {
		invalid = append(invalid, names...)
	}
	if len(invalid) > 0 {
		sort.Strings(invalid)
		fmt.Fprintf(w, "- no valid layer: %s\n", strings.Join(invalid, ", "))
	}
	if len(registry) == 0 {
		fmt.Fprint(w, "(none)\n")
	}

	fmt.Fprint(w, "\n## Rules\n")
	for _, d := range eng.RuleDocs() {
		desc := ""
		if d.Description != "" {
			desc = ": " + d.Description
		}
		fmt.Fprintf(w, "- %s (%s) %s%s\n", d.ID, d.Severity, d.Title, desc)
	}
	return nil
}
