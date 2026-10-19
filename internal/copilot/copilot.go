// Package copilot gives the GitHub Copilot review instructions that slint init
// writes and slint doctor checks.
package copilot

import (
	_ "embed" // For the go:embed directive.
	"path/filepath"
	"strings"
)

// InstructionsPath is the path of the review instructions, relative to the
// project directory. It is a file of its own, so that it does not change the
// .github/copilot-instructions.md of the project.
const InstructionsPath = ".github/instructions/speclint.instructions.md"

//go:embed templates/speclint.instructions.md
var instructions string

// Instructions returns the text of the review instructions. specsDir is the
// specs directory and registry is the registry file, relative to the
// project directory. Only the Markdown body uses them, so that a name with
// YAML syntax cannot make the frontmatter invalid.
func Instructions(specsDir, registry string) string {
	return strings.NewReplacer(
		"{{SPECS_DIR}}", filepath.ToSlash(specsDir),
		"{{REGISTRY}}", filepath.ToSlash(registry),
	).Replace(instructions)
}
