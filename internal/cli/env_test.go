package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommandsWithoutAWorkingDirectory covers a process whose working
// directory is not known, for example because it was deleted. Only the
// commands that need the directory fail.
func TestCommandsWithoutAWorkingDirectory(t *testing.T) {
	for _, c := range []struct {
		args  []string
		stdin string
		want  exitCode
	}{
		{[]string{"help"}, "", exitOK},
		{[]string{"schema"}, "", exitOK},
		{[]string{"hook"}, "not json", hookPass},
		{[]string{"hook"}, `{"tool_input": {"file_path": "main.go"}}`, hookPass},
		{[]string{"hook"}, `{"tool_input": {"file_path": "specs/a.yaml"}}`, hookPass},
		{[]string{"check"}, "", exitError},
		{[]string{"context"}, "", exitError},
		{[]string{"init"}, "", exitError},
	} {
		var out, errOut bytes.Buffer
		env := &Env{Stdin: strings.NewReader(c.stdin), Stdout: &out, Stderr: &errOut}
		if got := exitCode(Run(context.Background(), c.args, env)); got != c.want {
			t.Errorf("%v: exit %d, want %d (stderr %q)", c.args, got, c.want, errOut.String())
		}
		if c.want == exitError && !strings.Contains(errOut.String(), "working directory") {
			t.Errorf("%v: stderr %q, want it to name the working directory", c.args, errOut.String())
		}
	}
}

// TestHookResolvesARelativePathAgainstEnvDir covers an event with no cwd.
// The hook must find the spec from Env.Dir, not from the directory of the
// process.
func TestHookResolvesARelativePathAgainstEnvDir(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		".specval.yaml":               "",
		".specval/data/services.yaml": "services:\n  OrderManager: { layer: manager }\n  OrderAccessor: { layer: accessor }\n",
		"specs/bad.yaml": "specval: 1\nid: A\nkind: feature\ntitle: T\nsummary: S\n" +
			"acceptance: [{ id: AC1, text: T }]\ncalls: [{ from: OrderAccessor, to: OrderManager, op: Notify, covers: [AC1] }]\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	env := &Env{Stdin: strings.NewReader(`{"tool_input": {"file_path": "specs/bad.yaml"}}`), Stdout: &out, Stderr: &errOut, Dir: dir}
	if got := exitCode(Run(context.Background(), []string{"hook"}, env)); got != hookBlock {
		t.Errorf("exit %d, want %d (stderr %q)", got, hookBlock, errOut.String())
	}
}
