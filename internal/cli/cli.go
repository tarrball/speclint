// Package cli implements the speclint commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/tarrball/speclint/internal/baseline"
)

// exitCode is the exit status of a command.
type exitCode int

// Exit codes of speclint.
const (
	exitOK       exitCode = 0 // The command succeeded.
	exitFindings exitCode = 1 // check found an error (or a warning with --strict), or doctor found a failure.
	exitError    exitCode = 2 // The command was not correct, or speclint could not do it.
)

// command is one command of slint. The usage, the dispatch, and --help come
// from the table of commands, so that they agree.
type command struct {
	name string
	// synopsis gives the arguments and flags, for the usage and --help.
	synopsis func() string
	// help is the description in the usage. Each line is one line of text.
	help []string
	// examples are command lines for --help. A test runs each one.
	examples []string
	run      func(ctx context.Context, env *Env, args []string) (exitCode, error)
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
			examples: []string{
				"slint check",
				"slint check specs/ORD-143.yaml --format json",
			},
			run: cmdCheck,
		},
		{
			name: "context",
			help: []string{
				"Print the spec format, the layers, the registry, and the rules.",
				"An agent reads this text before it writes a spec.",
			},
			examples: []string{"slint context"},
			run:      noCode(cmdContext),
		},
		{
			name: "new",
			synopsis: func() string {
				return `<id> [--kind ` + strings.Join(baseline.Kinds(), "|") + `] [--title "..."]`
			},
			help:     []string{"Make the spec <specs dir>/<id>.yaml from the template."},
			examples: []string{`slint new ORD-143 --kind bugfix --title "Refund a partial order"`},
			run:      noCode(cmdNew),
		},
		{
			name:     "schema",
			help:     []string{"Print the JSON Schema of a spec."},
			examples: []string{"slint schema"},
			run:      noCode(cmdSchema),
		},
		{
			name:     "init",
			synopsis: text("[--claude] [--dry-run]"),
			help: []string{
				"Set up speclint and the Copilot review instructions in this directory.",
				"With --claude, also add the Claude Code skill and hook.",
				"You can run it again. It adds only the parts that are missing.",
				"With --dry-run, it prints the changes and writes nothing.",
			},
			examples: []string{"slint init --claude --dry-run", "slint init --claude", "slint init"},
			run:      noCode(cmdInit),
		},
		{
			name:     "doctor",
			synopsis: text("[--format text|json]"),
			help: []string{
				"Check the setup: the configuration, the rules, the registry, the",
				"Claude Code skill and hook, the Copilot review instructions, and the",
				"slint binary on PATH.",
			},
			examples: []string{"slint doctor", "slint doctor --format json"},
			run:      cmdDoctor,
		},
		{
			name:     "version",
			help:     []string{"Print the version of slint and of the spec format."},
			examples: []string{"slint version"},
			run:      noCode(cmdVersion),
		},
		{
			name: "hook",
			help: []string{
				"Claude Code runs this command after each file write. Do not run it yourself.",
				"If the file is a spec, speclint checks it and sends the errors to Claude.",
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

// line returns the command with its synopsis, for example "slint doctor [--format text|json]".
func (c *command) line() string {
	if c.synopsis == nil {
		return "slint " + c.name
	}
	return "slint " + c.name + " " + c.synopsis()
}

// usage returns the usage text of slint.
func usage() string {
	var b strings.Builder
	b.WriteString("speclint checks change specs against architecture rules before you write code.\n\nUsage:\n")
	for _, c := range commands() {
		fmt.Fprintf(&b, "  %s\n", c.line())
		for _, h := range c.help {
			fmt.Fprintf(&b, "        %s\n", h)
		}
	}
	b.WriteString("\nRun `slint <command> --help` for the flags and examples of a command.\n")
	return b.String()
}

// helpText returns the --help text of the command: the synopsis, the
// description, the flags in fs, and the examples. The usage of a flag is
// "description: `value`". The help puts the value after the name of the flag,
// and the description below it.
func (c *command) helpText(fs *flag.FlagSet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Usage: %s\n\n", c.line())
	for _, h := range c.help {
		fmt.Fprintf(&b, "%s\n", h)
	}
	var flags strings.Builder
	fs.VisitAll(func(f *flag.Flag) {
		value, usage := flag.UnquoteUsage(f)
		usage = strings.TrimSuffix(usage, ": "+value)
		fmt.Fprintf(&flags, "  --%s", f.Name)
		if value != "" {
			fmt.Fprintf(&flags, " %s", value)
		}
		fmt.Fprintf(&flags, "\n        %s", usage)
		if f.DefValue != "" && f.DefValue != "false" {
			fmt.Fprintf(&flags, " (default %s)", f.DefValue)
		}
		flags.WriteString("\n")
	})
	if flags.Len() > 0 {
		fmt.Fprintf(&b, "\nFlags:\n%s", flags.String())
	}
	if len(c.examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, e := range c.examples {
			fmt.Fprintf(&b, "  %s\n", e)
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
	// Executable is the path of the speclint binary that runs the command.
	// It is "" if the path is not known.
	Executable string
	// Version is the version of the binary, for example v0.1.0.
	Version string
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
	c := find(args[0])
	switch {
	case c != nil:
		code, err = c.run(ctx, env, args[1:])
	case args[0] == "help" && len(args) == 2 && find(args[1]) != nil:
		// slint help <command> is the same as slint <command> --help.
		c = find(args[1])
		code, err = c.run(ctx, env, []string{"--help"})
	case args[0] == "--version":
		// Many CLIs print their version with --version, so people and agents
		// try it first. It is the same as slint version.
		c = find("version")
		code, err = c.run(ctx, env, args[1:])
	case args[0] == "-h", args[0] == "--help", args[0] == "help" && len(args) == 1:
		fmt.Fprint(env.Stdout, usage())
	case args[0] == "help":
		fmt.Fprintf(env.Stderr, "unknown command %q\n\n%s", args[1], usage())
		code = exitError
	default:
		fmt.Fprintf(env.Stderr, "unknown command %q\n\n%s", args[0], usage())
		code = exitError
	}
	// For a flag that is not correct, the flag package has already written
	// the error.
	if fe, ok := errors.AsType[flagError](err); ok {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(env.Stdout, c.helpText(fe.fs))
			return exitOK
		}
		fmt.Fprintf(env.Stderr, "Usage: %s\nRun `slint %s --help` for the flags and examples.\n", c.line(), c.name)
		return exitError
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, "slint:", err)
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
	fl.Var(&f, "format", "the output format: `text|json`")
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
			return nil, flagError{err, fs}
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

// flagError is an error from the flag package, which writes it to stderr
// itself. fs is the flag set, for the --help text.
type flagError struct {
	err error
	fs  *flag.FlagSet
}

func (e flagError) Error() string { return e.err.Error() }

func (e flagError) Unwrap() error { return e.err }

// newFlagSet returns a flag set that writes its messages to env.Stderr.
func newFlagSet(name string, env *Env) *flag.FlagSet {
	fl := flag.NewFlagSet(name, flag.ContinueOnError)
	fl.SetOutput(env.Stderr)
	// runCommand writes the help or the usage. It knows if the user asked
	// for help, and the flag package does not.
	fl.Usage = func() {}
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
