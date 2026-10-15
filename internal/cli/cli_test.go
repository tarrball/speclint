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

	"github.com/tarrball/specval/internal/doctor"
	"github.com/tarrball/specval/internal/policy"
)

// runCLI runs specval with args in dir and returns the exit code, stdout,
// and stderr.
func runCLI(t *testing.T, dir, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Run(context.Background(), args, &Env{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out,
		Stderr: &errOut,
		Dir:    dir,
	})
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
		if code != exitError || !strings.HasPrefix(errOut, "specval: no .specval.yaml found") {
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
		want int
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

func TestReportAndPrintText(t *testing.T) {
	r := &Report{}
	r.add("a.yaml", nil)
	r.add("b.yaml", []policy.Finding{
		{Rule: "SV101", Severity: policy.SeverityError, Path: "calls[0].to", Line: 3, Msg: "X is not registered", Fix: "Register X."},
		{Rule: "SV201", Severity: policy.SeverityWarning, Line: 5, Msg: "Y is unused", Fix: "Remove Y."},
	})
	if r.Errors != 1 || r.Warnings != 1 || len(r.Files) != 2 || r.Files[0].Findings == nil {
		t.Errorf("report = %+v", r)
	}
	var b strings.Builder
	printText(&b, r, 2)
	want := `b.yaml:3 error SV101 X is not registered (calls[0].to)
    fix: Register X.
b.yaml:5 warning SV201 Y is unused
    fix: Remove Y.
2 spec(s) checked: 1 error(s), 1 warning(s)
`
	if b.String() != want {
		t.Errorf("printText:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestPrintTextWithoutALine(t *testing.T) {
	r := &Report{}
	r.add("(data)", []policy.Finding{{Rule: "PRJ900", Severity: policy.SeverityWarning, Path: "x", Msg: "m", Fix: "f"}})
	var b strings.Builder
	printText(&b, r, 0)
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
1 problem(s), 1 warning(s).
`
	if b.String() != want {
		t.Errorf("printChecks:\n%s\nwant:\n%s", b.String(), want)
	}
	b.Reset()
	printChecks(&b, []doctor.Check{{Status: doctor.StatusOK, Name: "config", Detail: "x"}})
	if !strings.HasSuffix(b.String(), "specval setup looks good.\n") {
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
	if code, _, errOut := runCLI(t, dir, "", "new", "bad id"); code != exitError || !strings.Contains(errOut, "usage: specval new <id>") {
		t.Errorf("new with a bad id: code %d, stderr %q", code, errOut)
	}
}
