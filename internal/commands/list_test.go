package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestListCreate_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newListCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"create", "--title", "Sprint backlog"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "slackLists.create") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}

func TestListCommand_HasSubcommands(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{})
	want := map[string]bool{"create": false, "read": false, "add-item": false, "update-item": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing list subcommand %q", name)
		}
	}
}
