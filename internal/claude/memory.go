package claude

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MemoryLine is the line that slint init tells the user to add to CLAUDE.md.
// The skill loads only when Claude thinks that the task needs it. This line
// tells Claude to use it for each change.
const MemoryLine = "This project uses speclint. Before you write code for a change, use the spec skill to write a spec that passes `slint check`."

// memoryPaths are the project memory files of Claude Code, relative to the
// project directory.
var memoryPaths = []string{"CLAUDE.md", ".claude/CLAUDE.md", "CLAUDE.local.md"}

// MemoryHint returns MemoryLine if no project memory file in dir contains
// the word speclint, and "" if one does. It does not change the files.
func MemoryHint(dir string) (string, error) {
	for _, p := range memoryPaths {
		b, err := os.ReadFile(filepath.Join(dir, p))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if strings.Contains(string(b), "speclint") {
			return "", nil
		}
	}
	return MemoryLine, nil
}
