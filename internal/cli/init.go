package cli

import (
	"context"

	"github.com/tarrball/speclint/internal/scaffold"
)

func cmdInit(_ context.Context, env *Env, args []string) error {
	fl := newFlagSet("init", env)
	dry := fl.Bool("dry-run", false, "print the changes, and write nothing")
	if err := parseNoArgs(fl, args); err != nil {
		return err
	}
	dir, err := env.workDir()
	if err != nil {
		return err
	}
	if *dry {
		return scaffold.DryRun(dir, env.Stdout)
	}
	return scaffold.Init(dir, env.Stdout)
}
