package claude

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestReadEvent(t *testing.T) {
	ev, err := ReadEvent(strings.NewReader(`{"cwd": "/p", "tool_name": "Write", "tool_input": {"file_path": "specs/a.yaml", "content": "x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if ev.Cwd != "/p" || ev.ToolInput.FilePath != "specs/a.yaml" {
		t.Errorf("ReadEvent = %+v", ev)
	}
	if _, err := ReadEvent(strings.NewReader("not json")); err == nil {
		t.Error("ReadEvent(not json): want an error")
	}
}

func TestWriteContext(t *testing.T) {
	var b bytes.Buffer
	if err := WriteContext(&b, "line 1\nline 2"); err != nil {
		t.Fatal(err)
	}
	var out struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(b.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.HookSpecificOutput.HookEventName != "PostToolUse" || out.HookSpecificOutput.AdditionalContext != "line 1\nline 2" {
		t.Errorf("WriteContext wrote %s", b.String())
	}
}
