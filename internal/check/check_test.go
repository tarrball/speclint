package check

import (
	"strings"
	"testing"

	"github.com/tarrball/specval/internal/policy"
)

func TestReportAndWriteText(t *testing.T) {
	r := &Report{Specs: 2}
	r.add("a.yaml", nil)
	r.add("b.yaml", []policy.Finding{
		{Rule: "SV101", Severity: policy.SeverityError, Path: "calls[0].to", Line: 3, Msg: "X is not registered", Fix: "Register X."},
		{Rule: "SV201", Severity: policy.SeverityWarning, Line: 5, Msg: "Y is unused", Fix: "Remove Y."},
	})
	if r.Errors != 1 || r.Warnings != 1 || len(r.Files) != 2 || r.Files[0].Findings == nil {
		t.Errorf("report = %+v", r)
	}
	var b strings.Builder
	r.WriteText(&b)
	want := `b.yaml:3 error SV101 X is not registered (calls[0].to)
    fix: Register X.
b.yaml:5 warning SV201 Y is unused
    fix: Remove Y.
specval checked 2 spec(s) and found 1 error(s) and 1 warning(s).
`
	if b.String() != want {
		t.Errorf("WriteText:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestWriteTextWithoutALine(t *testing.T) {
	r := &Report{}
	r.add("(data)", []policy.Finding{{Rule: "PRJ900", Severity: policy.SeverityWarning, Path: "x", Msg: "m", Fix: "f"}})
	var b strings.Builder
	r.WriteText(&b)
	if want := "(data) warning PRJ900 m (x)\n"; !strings.HasPrefix(b.String(), want) {
		t.Errorf("WriteText:\n%s\nwant it to start with:\n%s", b.String(), want)
	}
}

func TestProjectAndHookEventWithoutAConfiguration(t *testing.T) {
	dir := t.TempDir()
	if _, err := Project(t.Context(), dir, nil); err == nil {
		t.Error("Project with no configuration: no error")
	}
	var out, errOut strings.Builder
	status, err := HookEvent(t.Context(), HookEnv{
		Stdin:  strings.NewReader(`{"cwd": "` + dir + `", "tool_input": {"file_path": "specs/a.yaml"}}`),
		Stdout: &out,
		Stderr: &errOut,
		Dir:    dir,
	})
	if status != HookPass || err != nil || out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("HookEvent: status %d, err %v, stdout %q, stderr %q", status, err, out.String(), errOut.String())
	}
}
