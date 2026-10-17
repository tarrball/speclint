package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/tarrball/specval/internal/baseline"
	"github.com/tarrball/specval/internal/describe"
)

func cmdContext(ctx context.Context, env *Env, args []string) error {
	if err := parseNoArgs(newFlagSet("context", env), args); err != nil {
		return err
	}
	dir, err := env.workDir()
	if err != nil {
		return err
	}
	p, err := describe.Find(ctx, dir)
	if err != nil {
		return err
	}
	arch, registry := p.Architecture, p.Services
	w := env.Stdout

	fmt.Fprintf(w, "# Spec rules (specval)\n\nEach change needs a spec at %s/<id>.yaml, and the spec must pass before you write code. To make a new spec, run `sv new <id>`. To check a spec, run `sv check <file>`. Each finding has a fix.\n\n", p.SpecsDir)
	fmt.Fprintf(w, "## Format\n```yaml\n%s```\n\n", baseline.Format())

	fmt.Fprint(w, "## Layers\nEach layer can call only the layers after its arrow.\n")
	var suffixes, noSuffix []string
	for _, l := range arch.LayerOrder {
		targets := strings.Join(arch.Layers[l], ", ")
		if targets == "" {
			targets = "nothing"
		}
		fmt.Fprintf(w, "- %s -> %s\n", l, targets)
		if s := arch.Suffix[l]; s != "" {
			suffixes = append(suffixes, s)
		} else {
			noSuffix = append(noSuffix, l)
		}
	}
	fmt.Fprintf(w, "The name of a service ends with the suffix of its layer: %s.\n", strings.Join(suffixes, ", "))
	if len(noSuffix) > 0 {
		fmt.Fprintf(w, "A service in these layers has no suffix: %s.\n", strings.Join(noSuffix, ", "))
	}
	fmt.Fprintln(w)

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
	printLimits(w, registry)

	fmt.Fprint(w, "\n## Rules\n")
	for _, d := range p.Rules {
		desc := ""
		if d.Description != "" {
			desc = ": " + d.Description
		}
		fmt.Fprintf(w, "- %s (%s) %s%s\n", d.ID, d.Severity, d.Title, desc)
	}
	return nil
}

// printLimits writes the limits of the registry. A value in a limit is a
// layer or a service.
func printLimits(w io.Writer, registry map[string]describe.Service) {
	var lines []string
	for _, name := range slices.Sorted(maps.Keys(registry)) {
		s := registry[name]
		if s.MayCall != nil {
			lines = append(lines, fmt.Sprintf("- %s can call only: %s", name, listOrNothing(s.MayCall)))
		}
		if s.CalledBy != nil {
			lines = append(lines, fmt.Sprintf("- %s can be called only by: %s", name, listOrNothing(s.CalledBy)))
		}
	}
	if len(lines) > 0 {
		fmt.Fprintf(w, "\nThese limits make the layer rules stricter:\n%s\n", strings.Join(lines, "\n"))
	}
}

func listOrNothing(xs []string) string {
	if len(xs) == 0 {
		return "nothing"
	}
	return strings.Join(xs, ", ")
}
