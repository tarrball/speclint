// Package cli implements the specval commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
)

// exitCode is the exit status of a command.
type exitCode int

// Exit codes of specval.
const (
	exitOK       exitCode = 0 // The command succeeded.
	exitFindings exitCode = 1 // check found an error (or a warning with --strict), or doctor found a failure.
	exitError    exitCode = 2 // The command was not correct, or specval could not do it.
)

const usage = `specval checks change specs against architecture rules before you write code.

Usage:
  specval check [--format text|json] [--strict] [files...]
        Check the given specs, or each spec in the specs directory.
        The exit status is 1 if a spec has an error, or a warning with --strict.
  specval context
        Print the spec format, the layers, the registry, and the rules.
        An agent reads this text before it writes a spec.
  specval new <id> [--kind feature|bugfix|chore] [--title "..."]
        Make the spec <specs dir>/<id>.yaml from the template.
  specval schema
        Print the JSON Schema of a spec.
  specval init
        Set up specval, the Claude Code skill, and the hook in this directory.
        You can run it again. It adds only the parts that are missing.
  specval doctor [--format text|json]
        Check the setup: the configuration, the rules, the registry, the
        Claude Code skill and hook, and the specval binary on PATH.
  specval hook
        Claude Code runs this command after each file write. Do not run it yourself.
        If the file is a spec, specval checks it and sends the errors to Claude.
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
	return int(runCommand(ctx, args, env))
}

// runCommand runs the command in args and returns its exit code.
func runCommand(ctx context.Context, args []string, env *Env) exitCode {
	if len(args) < 1 {
		fmt.Fprint(env.Stderr, usage)
		return exitError
	}
	rest := args[1:]
	var code exitCode
	var err error
	switch args[0] {
	case "check":
		code, err = cmdCheck(ctx, env, rest)
	case "context":
		err = cmdContext(ctx, env, rest)
	case "new":
		err = cmdNew(env, rest)
	case "schema":
		err = cmdSchema(env, rest)
	case "init":
		err = cmdInit(env, rest)
	case "doctor":
		code, err = cmdDoctor(ctx, env, rest)
	case "hook":
		if err = parseNoArgs(newFlagSet("hook", env), rest); err == nil {
			code = cmdHook(ctx, env)
		}
	case "-h", "--help", "help":
		fmt.Fprint(env.Stdout, usage)
	default:
		fmt.Fprintf(env.Stderr, "unknown command %q\n\n%s", args[0], usage)
		code = exitError
	}
	// The flag package has already written the usage for --help, and the
	// error and the usage for a flag that is not correct.
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if _, ok := errors.AsType[flagError](err); ok {
		return exitError
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

// outputFormat is the format of the output of check and doctor.
type outputFormat string

// The values of outputFormat.
const (
	formatText outputFormat = "text"
	formatJSON outputFormat = "json"
)

// String returns the format as text, for the flag package.
func (f *outputFormat) String() string { return string(*f) }

// Set sets the format from a flag value. Thus an unknown format stops the
// command before it does any work.
func (f *outputFormat) Set(s string) error {
	switch v := outputFormat(s); v {
	case formatText, formatJSON:
		*f = v
		return nil
	default:
		return fmt.Errorf("unknown format %q; use text or json", s)
	}
}

// formatFlag adds the --format flag to fl.
func formatFlag(fl *flag.FlagSet) *outputFormat {
	f := formatText
	fl.Var(&f, "format", "output format: text or json")
	return &f
}

// writeJSON writes v to w as indented JSON. It does not escape <, >, and &,
// because the output is not HTML, and the messages can contain them.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// parseArgs parses flags that come before or after the positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, flagError{err}
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

// synopses gives the arguments of each command, for the usage of its flags.
var synopses = map[string]string{
	"check":  "[--format text|json] [--strict] [files...]",
	"new":    `<id> [--kind feature|bugfix|chore] [--title "..."]`,
	"doctor": "[--format text|json]",
}

// flagError is an error from the flag package, which writes it to stderr
// itself.
type flagError struct{ err error }

func (e flagError) Error() string { return e.err.Error() }

func (e flagError) Unwrap() error { return e.err }

// newFlagSet returns a flag set that writes its messages to env.Stderr.
func newFlagSet(name string, env *Env) *flag.FlagSet {
	fl := flag.NewFlagSet(name, flag.ContinueOnError)
	fl.SetOutput(env.Stderr)
	fl.Usage = func() {
		fmt.Fprintf(fl.Output(), "Usage: specval %s %s\n", name, synopses[name])
		fl.PrintDefaults()
	}
	return fl
}

// parseNoArgs parses the flags of a command that takes no positional
// arguments, and returns an error if there is one.
func parseNoArgs(fl *flag.FlagSet, args []string) error {
	pos, err := parseArgs(fl, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return fmt.Errorf("%s takes no arguments; got %q", fl.Name(), pos[0])
	}
	return nil
}
