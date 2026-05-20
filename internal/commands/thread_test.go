package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestThreadRead_FlagsRegistered(t *testing.T) {
	g := &GlobalFlags{}
	cmd := newThreadCommand(g)
	read, _, err := cmd.Find([]string{"read"})
	if err != nil {
		t.Fatalf("find read: %v", err)
	}
	for _, name := range []string{"oldest", "latest", "cursor", "limit", "channel", "thread"} {
		if read.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s on thread read", name)
		}
	}
	if read.Flags().Lookup("ts") != nil {
		t.Error("--ts must be removed; both thread verbs use --thread (no alias)")
	}
}

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
