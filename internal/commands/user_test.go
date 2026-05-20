package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/howar31/slk/internal/api"
)

func TestUserList_FlagsRegistered(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	list, _, err := cmd.Find([]string{"list"})
	if err != nil {
		t.Fatalf("find list: %v", err)
	}
	for _, name := range []string{"limit", "cursor", "include-bots", "include-deactivated"} {
		if list.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s on user list", name)
		}
	}
}

func TestFetchUsersWith_DefaultFilterAndPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"members": []map[string]any{
				{"id": "U1", "name": "alice", "real_name": "Alice"},
				{"id": "U2", "name": "bot1", "real_name": "Bot 1", "is_bot": true},
				{"id": "U3", "name": "ghost", "real_name": "Ghost", "deleted": true},
				{"id": "U4", "name": "bob", "real_name": "Bob"},
			},
			"response_metadata": map[string]any{"next_cursor": ""},
		})
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	hits, err := fetchUsersWith(c, userListOpts{})
	if err != nil {
		t.Fatalf("default filter: %v", err)
	}
	// Default: drop bots + deactivated. Expect U1 and U4 only.
	if len(hits) != 2 {
		t.Fatalf("default filter should yield 2 hits, got %d (%+v)", len(hits), hits)
	}
	for _, h := range hits {
		if h.ID == "U2" || h.ID == "U3" {
			t.Errorf("filter let through forbidden ID %s", h.ID)
		}
	}

	allHits, err := fetchUsersWith(c, userListOpts{IncludeBots: true, IncludeDeactivated: true})
	if err != nil {
		t.Fatalf("include-all: %v", err)
	}
	if len(allHits) != 4 {
		t.Errorf("include-all should yield 4 hits, got %d", len(allHits))
	}

	capped, err := fetchUsersWith(c, userListOpts{Limit: 1})
	if err != nil {
		t.Fatalf("limit: %v", err)
	}
	if len(capped) != 1 {
		t.Errorf("limit not honored: got %d hits, want 1", len(capped))
	}
}

func TestFetchUsers_PreservesUnfilteredBehaviorForSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"members": []map[string]any{
				{"id": "U1", "name": "alice"},
				{"id": "B1", "name": "bot", "is_bot": true},
				{"id": "G1", "name": "ghost", "deleted": true},
			},
			"response_metadata": map[string]any{"next_cursor": ""},
		})
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	hits, err := fetchUsers(c)
	if err != nil {
		t.Fatalf("fetchUsers: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("search.users path should keep bots+deactivated; got %d hits", len(hits))
	}
}

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
