package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain lets each script run the test binary as the specval command.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){"specval": main})
}

// TestScripts runs the end-to-end CLI tests in testdata/script.
// Each script starts in an empty directory. $EXAMPLES is the examples directory.
// Set UPDATE_SCRIPTS=1 to write the actual output into the scripts.
func TestScripts(t *testing.T) {
	examples, err := filepath.Abs("examples")
	if err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testscript.Params{
		Dir:                 filepath.Join("testdata", "script"),
		RequireExplicitExec: true,
		Setup: func(env *testscript.Env) error {
			env.Setenv("EXAMPLES", examples)
			return nil
		},
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"exits":  cmdExits,
			"expand": cmdExpand,
		},
		UpdateScripts: os.Getenv("UPDATE_SCRIPTS") == "1",
	})
}

// cmdExits implements "exits <code> <command> [args...]". It runs the command
// like exec, and fails the script if the exit code is not <code>.
func cmdExits(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 2 {
		ts.Fatalf("usage: exits <code> <command> [args...]")
	}
	want, err := strconv.Atoi(args[0])
	if err != nil {
		ts.Fatalf("exits: bad exit code %q", args[0])
	}
	got := 0
	if err := ts.Exec(args[1], args[2:]...); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			ts.Fatalf("exits: %v", err)
		}
		got = exitErr.ExitCode()
	}
	if got != want {
		ts.Fatalf("%s exited with code %d, want %d", args[1], got, want)
	}
}

// cmdExpand implements "expand <src> <dst>". It copies src to dst and replaces
// each $VAR or ${VAR} with the value of that script variable.
func cmdExpand(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: expand <src> <dst>")
	}
	out := os.Expand(ts.ReadFile(args[0]), ts.Getenv)
	ts.Check(os.WriteFile(ts.MkAbs(args[1]), []byte(out), 0o644))
}
