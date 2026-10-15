package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/tarrball/specval/internal/doctor"
)

func cmdDoctor(ctx context.Context, env *Env, args []string) (exitCode, error) {
	fl := newFlagSet("doctor", env)
	format := formatFlag(fl)
	if err := parseNoArgs(fl, args); err != nil {
		return exitError, err
	}
	dir, err := env.workDir()
	if err != nil {
		return exitError, err
	}
	checks := doctor.Run(ctx, doctor.Env{Dir: dir, Home: env.Home, Executable: env.Executable})
	failed := false
	for _, c := range checks {
		failed = failed || c.Status == doctor.StatusFail
	}
	switch *format {
	case formatJSON:
		if err := writeJSON(env.Stdout, doctorReport{Checks: checks, OK: !failed}); err != nil {
			return exitError, err
		}
	case formatText:
		printChecks(env.Stdout, checks)
	}
	if failed {
		return exitFindings, nil
	}
	return exitOK, nil
}

// doctorReport is the JSON output of doctor.
type doctorReport struct {
	Checks []doctor.Check `json:"checks"`
	// OK is false if a check failed.
	OK bool `json:"ok"`
}

func printChecks(w io.Writer, checks []doctor.Check) {
	marks := map[doctor.Status]string{doctor.StatusOK: "✓", doctor.StatusWarn: "!", doctor.StatusFail: "✗"}
	var warns, fails int
	for _, c := range checks {
		fmt.Fprintf(w, "%s %-9s %s\n", marks[c.Status], c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(w, "    fix: %s\n", c.Fix)
		}
		switch c.Status {
		case doctor.StatusWarn:
			warns++
		case doctor.StatusFail:
			fails++
		case doctor.StatusOK:
		}
	}
	if fails+warns == 0 {
		fmt.Fprintln(w, "The specval setup has no problems.")
	} else {
		fmt.Fprintf(w, "%d failure(s), %d warning(s).\n", fails, warns)
	}
}
