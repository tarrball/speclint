package cli

import (
	"context"

	"github.com/tarrball/specval/internal/scaffold"
)

func cmdInit(_ context.Context, env *Env, args []string) error {
	if err := parseNoArgs(newFlagSet("init", env), args); err != nil {
		return err
	}
	dir, err := env.workDir()
	if err != nil {
		return err
	}
	return scaffold.Init(dir, env.Stdout)
}
