package cli

import (
	"context"

	"github.com/tarrball/specval/internal/check"
)

func cmdCheck(ctx context.Context, env *Env, args []string) (exitCode, error) {
	fl := newFlagSet("check", env)
	format := formatFlag(fl)
	strict := fl.Bool("strict", false, "fail on warnings too")
	files, err := parseArgs(fl, args)
	if err != nil {
		return exitError, err
	}
	dir, err := env.workDir()
	if err != nil {
		return exitError, err
	}
	r, err := check.Project(ctx, dir, files)
	if err != nil {
		return exitError, err
	}
	switch *format {
	case formatJSON:
		if err := writeJSON(env.Stdout, r); err != nil {
			return exitError, err
		}
	case formatText:
		r.WriteText(env.Stdout)
	}
	if r.Errors > 0 || (*strict && r.Warnings > 0) {
		return exitFindings, nil
	}
	return exitOK, nil
}
