package cli

import (
	"io"

	"github.com/tarrball/specval/internal/baseline"
)

func cmdSchema(env *Env) error {
	_, err := io.WriteString(env.Stdout, baseline.Schema())
	return err
}
