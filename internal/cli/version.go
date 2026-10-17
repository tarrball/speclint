package cli

import (
	"context"
	"fmt"

	"github.com/tarrball/specval/internal/baseline"
)

func cmdVersion(_ context.Context, env *Env, args []string) error {
	if err := parseNoArgs(newFlagSet("version", env), args); err != nil {
		return err
	}
	v := env.Version
	if v == "" {
		v = "devel"
	}
	_, err := fmt.Fprintf(env.Stdout, "sv %s (spec format %d)\n", v, baseline.FormatVersion())
	return err
}
