package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestThreadReply_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newThreadCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"reply", "--channel", "C1", "--thread", "1.0", "--text", "reply!"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "reply!") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}
