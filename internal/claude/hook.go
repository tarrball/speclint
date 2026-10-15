package claude

import (
	"encoding/json"
	"io"
)

// Event is the part of a Claude Code PostToolUse event that specval reads.
type Event struct {
	// Cwd is the working directory of the Claude Code session.
	Cwd       string `json:"cwd"`
	ToolInput struct {
		// FilePath is the file that the tool wrote. It can be relative to Cwd.
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

// ReadEvent decodes a hook event from r.
func ReadEvent(r io.Reader) (Event, error) {
	var ev Event
	err := json.NewDecoder(r).Decode(&ev)
	return ev, err
}

// WriteContext writes a PostToolUse hook result that gives text to Claude as
// additional context.
func WriteContext(w io.Writer, text string) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // The text can contain <, >, and &. It is not HTML.
	return enc.Encode(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "PostToolUse",
			"additionalContext": text,
		},
	})
}
