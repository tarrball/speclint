package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tarrball/specval/internal/doctor"
)

func cmdDoctor(ctx context.Context, env *Env, args []string) (int, error) {
	fl := newFlagSet("doctor", env)
	format := fl.String("format", "text", "output format: text or json")
	if _, err := parseArgs(fl, args); err != nil {
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
	case "json":
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"checks": checks, "ok": !failed}); err != nil {
			return exitError, err
		}
	case "text":
		printChecks(env.Stdout, checks)
	default:
		return exitError, fmt.Errorf("unknown format %q", *format)
	}
	if failed {
		return exitFindings, nil
	}
	return exitOK, nil
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
		fmt.Fprintln(w, "specval setup looks good.")
	} else {
		fmt.Fprintf(w, "%d problem(s), %d warning(s).\n", fails, warns)
	}
}
