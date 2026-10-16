package cli

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tarrball/specval/internal/check"
	"github.com/tarrball/specval/internal/doctor"
	"github.com/tarrball/specval/internal/policy"
)

// runCLI runs specval with args in dir and returns the exit code, stdout,
// and stderr.
func runCLI(t *testing.T, dir, stdin string, args ...string) (code exitCode, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = exitCode(Run(context.Background(), args, &Env{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out,
		Stderr: &errOut,
		Dir:    dir,
	}))
	return code, out.String(), errOut.String()
}

func TestRunUsage(t *testing.T) {
	code, out, errOut := runCLI(t, t.TempDir(), "")
	if code != exitError || out != "" || errOut != usage {
		t.Errorf("no command: code %d, stdout %q, stderr %q", code, out, errOut)
	}
	for _, arg := range []string{"help", "-h", "--help"} {
		code, out, errOut := runCLI(t, t.TempDir(), "", arg)
		if code != exitOK || out != usage || errOut != "" {
			t.Errorf("%s: code %d, stdout %q, stderr %q", arg, code, out, errOut)
		}
	}
	code, _, errOut = runCLI(t, t.TempDir(), "", "nope")
	if code != exitError || !strings.HasPrefix(errOut, `unknown command "nope"`) {
		t.Errorf("unknown command: code %d, stderr %q", code, errOut)
	}
}

func TestRunReportsErrors(t *testing.T) {
	// Each command that needs a project fails in a directory without one.
	for _, cmd := range []string{"check", "context", "new"} {
		args := []string{cmd}
		if cmd == "new" {
			args = append(args, "X-1")
		}
		code, _, errOut := runCLI(t, t.TempDir(), "", args...)
		if code != exitError || !strings.HasPrefix(errOut, "sv: no .specval.yaml in ") {
			t.Errorf("%s: code %d, stderr %q", cmd, code, errOut)
		}
	}
	// A flag error goes to Env.Stderr.
	code, _, errOut := runCLI(t, t.TempDir(), "", "check", "--nope")
	if code != exitError || !strings.Contains(errOut, "flag provided but not defined: -nope") {
		t.Errorf("bad flag: code %d, stderr %q", code, errOut)
	}
}

func TestCheckExitCodes(t *testing.T) {
	examples, err := filepath.Abs(filepath.Join("..", "..", "examples", "refund"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want exitCode
	}{
		{[]string{"check", "specs/ORD-142.yaml"}, exitOK},
		{[]string{"check", "specs/ORD-143.yaml"}, exitFindings},
		{[]string{"check"}, exitFindings},
		{[]string{"check", "--format", "xml"}, exitError},
		{[]string{"check", "specs/missing.yaml"}, exitError},
	} {
		if code, _, errOut := runCLI(t, examples, "", c.args...); code != c.want {
			t.Errorf("%v: code %d, want %d (stderr %q)", c.args, code, c.want, errOut)
		}
	}
}

func TestCheckRejectsSpecsFromTwoProjects(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"a", "b", "a/sub"} {
		if err := os.MkdirAll(filepath.Join(dir, p, "specs"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, p, ".specval.yaml"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"a/specs/x.yaml", "a/sub/specs/z.yaml", "b/specs/y.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, p), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errOut := runCLI(t, dir, "", "check", "a/specs/x.yaml", "b/specs/y.yaml")
	if code != exitError || out != "" || !strings.Contains(errOut, "a/specs/x.yaml and b/specs/y.yaml are in different specval projects") {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
	// check accepts files in two directories of the same project.
	if code, _, errOut := runCLI(t, dir, "", "check", "a/specs/x.yaml", "a/sub/specs/z.yaml"); code != exitFindings {
		t.Errorf("same project: code %d, stderr %q", code, errOut)
	}
	// A symbolic link to the same project is the same project.
	if err := os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runCLI(t, dir, "", "check", "a/specs/x.yaml", "alias/specs/x.yaml"); code != exitFindings {
		t.Errorf("symbolic link: code %d, stderr %q", code, errOut)
	}
}

func TestSchema(t *testing.T) {
	code, out, _ := runCLI(t, t.TempDir(), "", "schema")
	if code != exitOK || !strings.Contains(out, `"title": "specval spec"`) {
		t.Errorf("schema: code %d, stdout %q", code, out)
	}
}

func TestHookIgnoresInputThatIsNotAboutASpec(t *testing.T) {
	dir := t.TempDir()
	for _, stdin := range []string{
		"",
		"not json",
		`{"tool_input": {}}`,
		`{"tool_input": {"file_path": "main.go"}}`,
		`{"cwd": "` + dir + `", "tool_input": {"file_path": "specs/a.yaml"}}`,
	} {
		code, out, errOut := runCLI(t, dir, stdin, "hook")
		if code != hookPass || out != "" || errOut != "" {
			t.Errorf("stdin %q: code %d, stdout %q, stderr %q", stdin, code, out, errOut)
		}
	}
}

func TestParseArgs(t *testing.T) {
	for _, c := range []struct {
		args    []string
		wantPos []string
		wantFmt string
		wantErr bool
	}{
		{nil, nil, "text", false},
		{[]string{"a", "b"}, []string{"a", "b"}, "text", false},
		{[]string{"--format", "json", "a"}, []string{"a"}, "json", false},
		{[]string{"a", "--format", "json", "b"}, []string{"a", "b"}, "json", false},
		{[]string{"a", "--format=json"}, []string{"a"}, "json", false},
		{[]string{"--", "--format"}, []string{"--format"}, "text", false},
		{[]string{"a", "--nope"}, nil, "", true},
	} {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		format := fs.String("format", "text", "")
		pos, err := parseArgs(fs, c.args)
		if (err != nil) != c.wantErr {
			t.Errorf("%v: err = %v", c.args, err)
			continue
		}
		if !c.wantErr && (!slices.Equal(pos, c.wantPos) || *format != c.wantFmt) {
			t.Errorf("%v: pos %v, format %q; want %v, %q", c.args, pos, *format, c.wantPos, c.wantFmt)
		}
	}
}

func TestPrintText(t *testing.T) {
	r := &check.Report{
		Files: []check.FileReport{
			{File: "a.yaml", Findings: []policy.Finding{}},
			{File: "b.yaml", Findings: []policy.Finding{
				{Rule: "SV101", Severity: policy.SeverityError, Path: "calls[0].to", Line: 3, Msg: "X is not registered", Fix: "Register X."},
				{Rule: "SV201", Severity: policy.SeverityWarning, Line: 5, Msg: "Y is unused", Fix: "Remove Y."},
			}},
		},
		Errors:   1,
		Warnings: 1,
		Specs:    2,
	}
	var b strings.Builder
	printText(&b, r)
	want := `b.yaml:3 error SV101 X is not registered (calls[0].to)
    fix: Register X.
b.yaml:5 warning SV201 Y is unused
    fix: Remove Y.
specval checked 2 spec(s) and found 1 error(s) and 1 warning(s).
`
	if b.String() != want {
		t.Errorf("printText:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestPrintTextWithoutALine(t *testing.T) {
	r := &check.Report{
		Files:    []check.FileReport{{File: "(data)", Findings: []policy.Finding{{Rule: "PRJ900", Severity: policy.SeverityWarning, Path: "x", Msg: "m", Fix: "f"}}}},
		Warnings: 1,
	}
	var b strings.Builder
	printText(&b, r)
	if want := "(data) warning PRJ900 m (x)\n"; !strings.HasPrefix(b.String(), want) {
		t.Errorf("printText:\n%s\nwant it to start with:\n%s", b.String(), want)
	}
}

func TestPrintChecks(t *testing.T) {
	var b strings.Builder
	printChecks(&b, []doctor.Check{
		{Status: doctor.StatusOK, Name: "config", Detail: "/p/.specval.yaml"},
		{Status: doctor.StatusWarn, Name: "registry", Detail: "empty", Fix: "Add services."},
		{Status: doctor.StatusFail, Name: "hook", Detail: "missing", Fix: "Run init."},
	})
	want := `✓ config    /p/.specval.yaml
! registry  empty
    fix: Add services.
✗ hook      missing
    fix: Run init.
1 failure(s), 1 warning(s).
`
	if b.String() != want {
		t.Errorf("printChecks:\n%s\nwant:\n%s", b.String(), want)
	}
	b.Reset()
	printChecks(&b, []doctor.Check{{Status: doctor.StatusOK, Name: "config", Detail: "x"}})
	if !strings.HasSuffix(b.String(), "The specval setup has no problems.\n") {
		t.Errorf("printChecks with no problems:\n%s", b.String())
	}
}

func TestInitAndNew(t *testing.T) {
	dir := t.TempDir()
	if code, _, errOut := runCLI(t, dir, "", "init"); code != exitOK {
		t.Fatalf("init: code %d, stderr %q", code, errOut)
	}
	code, out, errOut := runCLI(t, dir, "", "new", "A-1", "--kind", "chore")
	want := filepath.Join(dir, "specs", "A-1.yaml")
	if code != exitOK || out != want+"\n" {
		t.Fatalf("new: code %d, stdout %q, stderr %q", code, out, errOut)
	}
	if _, err := os.Stat(want); err != nil {
		t.Error(err)
	}
	if code, _, errOut := runCLI(t, dir, "", "new", "bad id"); code != exitError || !strings.Contains(errOut, "usage: sv new <id>") {
		t.Errorf("new with a bad id: code %d, stderr %q", code, errOut)
	}
}

func TestCheckJSONOutput(t *testing.T) {
	dir := t.TempDir()
	if code, _, errOut := runCLI(t, dir, "", "init"); code != exitOK {
		t.Fatalf("init: code %d, stderr %q", code, errOut)
	}
	// With nothing to report, files is an empty list, not null.
	if _, out, _ := runCLI(t, dir, "", "check", "--format", "json"); !strings.Contains(out, `"files": []`) {
		t.Errorf("check --format json with no specs:\n%s", out)
	}
	// The JSON output does not escape <, >, and & for HTML.
	spec := "specval: 1\nid: A\nkind: chore\ntitle: T\nsummary: S\nacceptance: [{ id: AC1, text: T }]\n" +
		"calls: [{ from: OrderManager, to: \"A<B&C>\", op: Get, covers: [AC1] }]\n"
	if err := os.WriteFile(filepath.Join(dir, "specs", "a.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, out, _ := runCLI(t, dir, "", "check", "--format", "json"); !strings.Contains(out, `"A<B&C> is not a registered service"`) {
		t.Errorf("check --format json escaped the message:\n%s", out)
	}
}

func TestHelpFlagExitsZeroAndDoesNothing(t *testing.T) {
	for _, cmd := range []string{"check", "context", "new", "schema", "init", "doctor", "hook"} {
		dir := t.TempDir()
		code, out, errOut := runCLI(t, dir, `{"tool_input": {"file_path": "specs/a.yaml"}}`, cmd, "--help")
		if code != exitOK || out != "" || !strings.Contains(errOut, "Usage: sv "+cmd) {
			t.Errorf("%s --help: code %d, stdout %q, stderr %q", cmd, code, out, errOut)
		}
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Errorf("%s --help wrote files: %v", cmd, entries)
		}
	}
}

func TestCommandsRejectExtraArguments(t *testing.T) {
	for _, cmd := range []string{"context", "schema", "init", "doctor", "hook"} {
		dir := t.TempDir()
		code, _, errOut := runCLI(t, dir, "", cmd, "extra")
		if code != exitError || !strings.Contains(errOut, cmd+` takes no arguments; got "extra"`) {
			t.Errorf("%s extra: code %d, stderr %q", cmd, code, errOut)
		}
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Errorf("%s extra wrote files: %v", cmd, entries)
		}
	}
}
