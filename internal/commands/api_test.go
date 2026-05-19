package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestAPICommand_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newAPICommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"conversations.list", "--params", `{"limit":"5"}`})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "conversations.list") {
		t.Fatalf("dry-run should echo the method, got %q", out.String())
	}
}
