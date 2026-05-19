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
	if got := out.String(); got == "" || !bytes.Contains(out.Bytes(), []byte("test-version")) {
		t.Fatalf("expected version output to contain test-version, got %q", got)
	}
}
