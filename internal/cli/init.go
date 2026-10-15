package cli

import (
	"os"

	"github.com/tarrball/specval/internal/scaffold"
)

func cmdInit() error {
	return scaffold.Init(".", os.Stdout)
}
