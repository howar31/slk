package commands

import (
	"strings"
	"testing"
)

func TestUserCommand_HasSubcommands(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	want := map[string]bool{"list": false, "info": false, "profile": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing user subcommand %q", name)
		}
	}
}

func TestUserProfile_Concise(t *testing.T) {
	p := userProfile{DisplayName: "Alice", RealName: "Alice Anderson", Title: "RD"}
	got := p.Concise()
	if !strings.Contains(got, "Alice") || !strings.Contains(got, "Alice Anderson") || !strings.Contains(got, "RD") {
		t.Fatalf("concise = %q", got)
	}
}
