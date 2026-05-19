package commands

import "testing"

func TestUserCommand_HasSubcommands(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	want := map[string]bool{"list": false, "info": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing user subcommand %q", name)
		}
	}
}
