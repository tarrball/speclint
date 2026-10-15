package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tarrball/specval/internal/doctor"
)

func cmdDoctor(ctx context.Context, args []string) (int, error) {
	fl := flag.NewFlagSet("doctor", flag.ContinueOnError)
	format := fl.String("format", "text", "output format: text or json")
	if _, err := parseArgs(fl, args); err != nil {
		return 2, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 2, err
	}
	checks := doctor.Run(ctx, cwd)
	failed := false
	for _, c := range checks {
		failed = failed || c.Status == "fail"
	}
	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"checks": checks, "ok": !failed}); err != nil {
			return 2, err
		}
	case "text":
		printChecks(os.Stdout, checks)
	default:
		return 2, fmt.Errorf("unknown format %q", *format)
	}
	if failed {
		return 1, nil
	}
	return 0, nil
}

func printChecks(w io.Writer, checks []doctor.Check) {
	marks := map[string]string{"ok": "✓", "warn": "!", "fail": "✗"}
	var warns, fails int
	for _, c := range checks {
		fmt.Fprintf(w, "%s %-9s %s\n", marks[c.Status], c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(w, "    fix: %s\n", c.Fix)
		}
		switch c.Status {
		case "warn":
			warns++
		case "fail":
			fails++
		}
	}
	if fails+warns == 0 {
		fmt.Fprintln(w, "specval setup looks good.")
	} else {
		fmt.Fprintf(w, "%d problem(s), %d warning(s).\n", fails, warns)
	}
}
