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

func cmdContext(ctx context.Context, env *Env) error {
	cfg, err := config.Find(env.Dir)
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

	fmt.Fprintf(w, "# Spec rules (specval)\n\nEvery change needs a passing spec at %s/<id>.yaml before code is written. Scaffold: `specval new <id>`. Validate: `specval check <file>`. Each finding includes a fix.\n\n", cfg.SpecsDir)
	fmt.Fprintf(w, "## Format\n```yaml\n%s```\n\n", baseline.Format())

	fmt.Fprint(w, "## Layers (caller -> may call)\n")
	var suffixes []string
	for _, l := range arch.LayerOrder {
		targets := strings.Join(arch.Layers[l], ", ")
		if targets == "" {
			targets = "nothing"
		}
		fmt.Fprintf(w, "- %s -> %s\n", l, targets)
		suffixes = append(suffixes, arch.Suffix[l])
	}
	fmt.Fprintf(w, "Service names end with their layer: %s.\n\n", strings.Join(suffixes, ", "))

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
	}
	if len(registry) == 0 {
		fmt.Fprint(w, "(none yet)\n")
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
