// Command specval validates change specs against architecture rules before
// a developer writes code.
package main

import (
	"context"
	"os"

	"github.com/tarrball/specval/internal/cli"
)

func main() {
	dir, _ := os.Getwd()        // If the directory is not known, Dir is "", and only the commands that need it fail.
	home, _ := os.UserHomeDir() // If there is no home directory, Home is "".
	exe, _ := os.Executable()   // If the path is not known, Executable is "".
	os.Exit(cli.Run(context.Background(), os.Args[1:], &cli.Env{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Dir:        dir,
		Home:       home,
		Executable: exe,
	}))
}
