// Command specval validates change specs against architecture rules before
// a developer writes code.
package main

import (
	"os"

	"github.com/tarrball/specval/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
