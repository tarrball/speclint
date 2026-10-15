package cli

import (
	"io"

	"github.com/tarrball/specval/internal/baseline"
)

func cmdSchema(env *Env, args []string) error {
	if err := parseNoArgs(newFlagSet("schema", env), args); err != nil {
		return err
	}
	_, err := io.WriteString(env.Stdout, baseline.Schema())
	return err
}
