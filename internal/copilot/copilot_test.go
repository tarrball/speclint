package copilot

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestInstructions(t *testing.T) {
	for _, dir := range []string{"specs", "changes: draft", "a #b"} {
		text := Instructions(dir, "arch/registry.yaml")
		parts := strings.SplitN(text, "---\n", 3)
		if len(parts) != 3 {
			t.Fatalf("%q: the instructions have no frontmatter", dir)
		}
		var front struct {
			ApplyTo      string `yaml:"applyTo"`
			ExcludeAgent string `yaml:"excludeAgent"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &front); err != nil || front.ApplyTo != "**" || front.ExcludeAgent != "coding-agent" {
			t.Errorf("%q: frontmatter %+v, err %v", dir, front, err)
		}
		if !strings.Contains(parts[2], "`"+dir+"/<id>.yaml`") || !strings.Contains(parts[2], "`arch/registry.yaml`") || strings.Contains(text, "{{") {
			t.Errorf("%q: the body does not name the specs directory and the registry:\n%s", dir, parts[2])
		}
	}
}

func TestInstructionsFitTheLimitOfCopilot(t *testing.T) {
	// Copilot code review reads only the first 4,000 characters of an
	// instructions file.
	if n := len(Instructions("specs", ".speclint/data/services.yaml")); n > 3500 {
		t.Errorf("the instructions have %d characters; keep them below 3500", n)
	}
}
