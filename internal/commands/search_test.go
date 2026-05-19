package commands

import "testing"

func TestSearchHit_Concise(t *testing.T) {
	h := searchHit{Name: "general", ID: "C1", Extra: "42 members"}
	got := h.Concise()
	if got == "" || got == "C1" {
		t.Fatalf("concise should combine fields, got %q", got)
	}
}

func TestSearchCommand_HasSubcommands(t *testing.T) {
	cmd := newSearchCommand(&GlobalFlags{})
	want := map[string]bool{"messages": false, "channels": false, "users": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing search subcommand %q", name)
		}
	}
}
