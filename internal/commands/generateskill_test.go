package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSkill_WritesFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "SKILL.md")

	root := NewRootCommand("9.9.9")
	root.SetArgs([]string{"generate-skill", "--output", out})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	content := string(data)
	for _, want := range []string{"name: slk", "version: 9.9.9", "## msg", "### slk channel invite"} {
		if !strings.Contains(content, want) {
			t.Errorf("generated skill missing %q", want)
		}
	}
}
