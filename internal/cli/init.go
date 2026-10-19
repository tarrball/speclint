package cli

import (
	"context"

	"github.com/tarrball/speclint/internal/scaffold"
)

func cmdInit(_ context.Context, env *Env, args []string) error {
	fl := newFlagSet("init", env)
	var opts scaffold.Options
	fl.BoolVar(&opts.Claude, "claude", false, "also add the Claude Code skill and hook")
	fl.BoolVar(&opts.DryRun, "dry-run", false, "print the changes, and write nothing")
	if err := parseNoArgs(fl, args); err != nil {
		return err
	}
	dir, err := env.workDir()
	if err != nil {
		return err
	}
	return scaffold.Init(dir, env.Stdout, opts)
}
