package commands

import "testing"

func TestSearchChannels_FlagsRegistered(t *testing.T) {
	cmd := newSearchCommand(&GlobalFlags{})
	ch, _, err := cmd.Find([]string{"channels"})
	if err != nil {
		t.Fatalf("find channels: %v", err)
	}
	for _, name := range []string{"query", "include-archived", "channel-types"} {
		if ch.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s on search channels", name)
		}
	}
}

func TestSearchUsers_QueryFlag(t *testing.T) {
	cmd := newSearchCommand(&GlobalFlags{})
	u, _, err := cmd.Find([]string{"users"})
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	if u.Flags().Lookup("query") == nil {
		t.Fatal("missing --query on search users")
	}
}

func TestSearchMessages_PublicFlag(t *testing.T) {
	cmd := newSearchCommand(&GlobalFlags{})
	m, _, err := cmd.Find([]string{"messages"})
	if err != nil {
		t.Fatalf("find messages: %v", err)
	}
	if m.Flags().Lookup("public") == nil {
		t.Fatal("missing --public on search messages")
	}
}

func TestSearchHit_Concise(t *testing.T) {
	h := searchHit{Name: "general", ID: "C1", Extra: "42 members"}
	want := "general (C1) — 42 members"
	if got := h.Concise(); got != want {
		t.Fatalf("Concise() = %q, want %q", got, want)
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
