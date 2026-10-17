// Package cli implements the specval commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/tarrball/specval/internal/baseline"
)

// exitCode is the exit status of a command.
type exitCode int

// Exit codes of specval.
const (
	exitOK       exitCode = 0 // The command succeeded.
	exitFindings exitCode = 1 // check found an error (or a warning with --strict), or doctor found a failure.
	exitError    exitCode = 2 // The command was not correct, or specval could not do it.
)

// command is one command of sv. The usage, the dispatch, and --help come
// from the table of commands, so that they agree.
type command struct {
	name string
	// synopsis gives the arguments and flags, for the usage and --help.
	synopsis func() string
	// help is the description in the usage. Each line is one line of text.
	help []string
	run  func(ctx context.Context, env *Env, args []string) (exitCode, error)
}

// noCode adapts a command that has no exit code of its own.
func noCode(f func(ctx context.Context, env *Env, args []string) error) func(context.Context, *Env, []string) (exitCode, error) {
	return func(ctx context.Context, env *Env, args []string) (exitCode, error) {
		return exitOK, f(ctx, env, args)
	}
}

func text(s string) func() string { return func() string { return s } }

// commands returns the table of commands, in the order of the usage. It is
// a function, not a variable, because the commands use the table for --help,
// and a variable that refers to itself through functions is an
// initialization cycle.
func commands() []command {
	return []command{
		{
			name:     "check",
			synopsis: text("[--format text|json] [--strict] [files...]"),
			help: []string{
				"Check the given specs, or each spec in the specs directory.",
				"The exit status is 1 if a spec has an error, or a warning with --strict.",
			},
			run: cmdCheck,
		},
		{
			name: "context",
			help: []string{
				"Print the spec format, the layers, the registry, and the rules.",
				"An agent reads this text before it writes a spec.",
			},
			run: noCode(cmdContext),
		},
		{
			name: "new",
			synopsis: func() string {
				return `<id> [--kind ` + strings.Join(baseline.Kinds(), "|") + `] [--title "..."]`
			},
			help: []string{"Make the spec <specs dir>/<id>.yaml from the template."},
			run:  noCode(cmdNew),
		},
		{
			name: "schema",
			help: []string{"Print the JSON Schema of a spec."},
			run:  noCode(cmdSchema),
		},
		{
			name: "init",
			help: []string{
				"Set up specval, the Claude Code skill, and the hook in this directory.",
				"You can run it again. It adds only the parts that are missing.",
			},
			run: noCode(cmdInit),
		},
		{
			name:     "doctor",
			synopsis: text("[--format text|json]"),
			help: []string{
				"Check the setup: the configuration, the rules, the registry, the",
				"Claude Code skill and hook, and the sv binary on PATH.",
			},
			run: cmdDoctor,
		},
		{
			name: "hook",
			help: []string{
				"Claude Code runs this command after each file write. Do not run it yourself.",
				"If the file is a spec, specval checks it and sends the errors to Claude.",
			},
			run: func(ctx context.Context, env *Env, args []string) (exitCode, error) {
				if err := parseNoArgs(newFlagSet("hook", env), args); err != nil {
					return exitOK, err
				}
				return cmdHook(ctx, env), nil
			},
		},
	}
}

// find returns the command with the name, or nil.
func find(name string) *command {
	for _, c := range commands() {
		if c.name == name {
			return &c
		}
	}
	return nil
}

// line returns the command with its synopsis, for example "sv doctor [--format text|json]".
func (c *command) line() string {
	if c.synopsis == nil {
		return "sv " + c.name
	}
	return "sv " + c.name + " " + c.synopsis()
}

// usage returns the usage text of sv.
func usage() string {
	var b strings.Builder
	b.WriteString("specval checks change specs against architecture rules before you write code.\n\nUsage:\n")
	for _, c := range commands() {
		fmt.Fprintf(&b, "  %s\n", c.line())
		for _, h := range c.help {
			fmt.Fprintf(&b, "        %s\n", h)
		}
	}
	return b.String()
}

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
		fmt.Fprint(env.Stderr, usage())
		return exitError
	}
	var code exitCode
	var err error
	switch c := find(args[0]); {
	case c != nil:
		code, err = c.run(ctx, env, args[1:])
	case args[0] == "-h", args[0] == "--help", args[0] == "help":
		fmt.Fprint(env.Stdout, usage())
	default:
		fmt.Fprintf(env.Stderr, "unknown command %q\n\n%s", args[0], usage())
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
		fmt.Fprintln(env.Stderr, "sv:", err)
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
		fmt.Fprintf(fl.Output(), "Usage: %s\n", find(name).line())
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
