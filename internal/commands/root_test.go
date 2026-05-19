package commands

import (
	"bytes"
	"testing"
)

func TestRootCommand_Version(t *testing.T) {
	cmd := NewRootCommand("test-version")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.Bytes()
	if len(got) == 0 || !bytes.Contains(got, []byte("test-version")) {
		t.Fatalf("expected version output to contain test-version, got %q", got)
	}
}
