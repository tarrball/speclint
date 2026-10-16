package cli

import (
	"context"
	"fmt"
	"io"

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
		printText(env.Stdout, r)
	}
	if r.Errors > 0 || (*strict && r.Warnings > 0) {
		return exitFindings, nil
	}
	return exitOK, nil
}

// printText writes each finding and the totals to w.
func printText(w io.Writer, r *check.Report) {
	for _, fr := range r.Files {
		for _, f := range fr.Findings {
			where := fr.File
			if f.Line > 0 {
				where = fmt.Sprintf("%s:%d", fr.File, f.Line)
			}
			at := ""
			if f.Path != "" {
				at = " (" + f.Path + ")"
			}
			fmt.Fprintf(w, "%s %s %s %s%s\n    fix: %s\n", where, f.Severity, f.Rule, f.Msg, at, f.Fix)
		}
		if fr.Stopped {
			fmt.Fprintf(w, "%s note: The other rules run after you fix the format errors (SV000, SV001).\n", fr.File)
		}
	}
	fmt.Fprintf(w, "specval checked %d spec(s) and found %d error(s) and %d warning(s).\n", r.Specs, r.Errors, r.Warnings)
}
