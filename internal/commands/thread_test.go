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
	got := out.String()
	if !strings.Contains(got, "reply!") {
		t.Fatalf("dry-run output = %q: missing text 'reply!'", got)
	}
	if !strings.Contains(got, "C1") {
		t.Fatalf("dry-run output = %q: missing channel 'C1'", got)
	}
	if !strings.Contains(got, "thread_ts") {
		t.Fatalf("dry-run output = %q: missing 'thread_ts' param", got)
	}
}
