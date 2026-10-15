package cli

import (
	"io"

	"github.com/tarrball/specval/internal/baseline"
)

func cmdSchema(w io.Writer) error {
	_, err := io.WriteString(w, baseline.Schema())
	return err
}
