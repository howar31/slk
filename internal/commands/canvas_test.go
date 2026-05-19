package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestCanvasCreate_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newCanvasCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"create", "--title", "Plan", "--markdown", "# hi"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "canvases.create") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}
