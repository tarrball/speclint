// Package cli implements the specval commands.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
)

// Exit codes of specval.
const (
	exitOK       = 0 // The command succeeded.
	exitFindings = 1 // check found an error (or a warning with --strict), or doctor found a failure.
	exitError    = 2 // The command was not correct, or specval could not do it.
)

const usage = `specval validates specs against architecture rules before code is written.

Usage:
  specval check [--format text|json] [--strict] [files...]
        Validate specs (default: every spec in the specs dir). Exit 1 on errors.
  specval context
        Print the spec format, layer rules, registry, and rules (for an agent's context).
  specval new <id> [--kind feature|bugfix|chore] [--title "..."]
        Scaffold <specs dir>/<id>.yaml.
  specval schema
        Print the spec JSON Schema.
  specval init
        Set up specval and the Claude Code skill + hook in the current directory.
        Safe to re-run: it only adds what's missing.
  specval doctor [--format text|json]
        Check that the setup is intact (config, rules, registry, Claude skill, hook, PATH).
  specval hook
        Not run by hand: Claude Code runs it after each file write (set up by init).
        If the file is a spec, checks it and sends any errors back to Claude to fix.
`

// Env holds what a command reads from its process and writes to it.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Dir is the working directory, as an absolute path. It is "" if the
	// working directory is not known. Then a command that needs it fails.
	Dir string
	// Home is the home directory of the user.
	Home string
	// Executable is the path of the specval binary that runs the command.
	// It is "" if the path is not known.
	Executable string
}

// Run runs the command in args, which does not include the program name.
// It returns the exit code.
func Run(ctx context.Context, args []string, env *Env) int {
	if len(args) < 1 {
		fmt.Fprint(env.Stderr, usage)
		return exitError
	}
	rest := args[1:]
	var code int
	var err error
	switch args[0] {
	case "check":
		code, err = cmdCheck(ctx, env, rest)
	case "context":
		err = cmdContext(ctx, env)
	case "new":
		err = cmdNew(env, rest)
	case "schema":
		err = cmdSchema(env)
	case "init":
		err = cmdInit(env)
	case "doctor":
		code, err = cmdDoctor(ctx, env, rest)
	case "hook":
		code = cmdHook(ctx, env)
	case "-h", "--help", "help":
		fmt.Fprint(env.Stdout, usage)
	default:
		fmt.Fprintf(env.Stderr, "unknown command %q\n\n%s", args[0], usage)
		code = exitError
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, "specval:", err)
		if code == exitOK {
			code = exitError
		}
	}
	return code
}

// workDir returns env.Dir, or an error if the working directory is not known.
func (env *Env) workDir() (string, error) {
	if env.Dir == "" {
		return "", errors.New("the working directory is not known")
	}
	return env.Dir, nil
}

// parseArgs parses flags that come before or after the positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

// newFlagSet returns a flag set that writes its messages to env.Stderr.
func newFlagSet(name string, env *Env) *flag.FlagSet {
	fl := flag.NewFlagSet(name, flag.ContinueOnError)
	fl.SetOutput(env.Stderr)
	return fl
}
