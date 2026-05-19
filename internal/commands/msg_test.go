package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestMsgMessage_Concise(t *testing.T) {
	m := msgItem{User: "Bob", Text: "hi", TS: "1779191572.0"}
	if got := m.Concise(); !strings.Contains(got, "Bob") || !strings.Contains(got, "hi") {
		t.Fatalf("concise = %q", got)
	}
}

func TestMsgSend_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newMsgCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"send", "--channel", "C1", "--text", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "hello") || !strings.Contains(out.String(), "C1") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}
