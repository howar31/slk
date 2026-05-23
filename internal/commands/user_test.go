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
				{"id": "USLACKBOT", "name": "slackbot", "real_name": "Slackbot"},
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
	// Default: drop bots + deactivated + Slackbot. Expect U1 and U4 only.
	if len(hits) != 2 {
		t.Fatalf("default filter should yield 2 hits, got %d (%+v)", len(hits), hits)
	}
	for _, h := range hits {
		if h.ID == "U2" || h.ID == "U3" || h.ID == "USLACKBOT" {
			t.Errorf("filter let through forbidden ID %s", h.ID)
		}
	}

	allHits, err := fetchUsersWith(c, userListOpts{IncludeBots: true, IncludeDeactivated: true})
	if err != nil {
		t.Fatalf("include-all: %v", err)
	}
	if len(allHits) != 5 {
		t.Errorf("include-all should yield 5 hits, got %d", len(allHits))
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
	want := map[string]bool{
		"list":         false,
		"info":         false,
		"profile":      false,
		"by-email":     false,
		"presence":     false,
		"channels":     false,
		"set-profile":  false,
		"set-photo":    false,
		"delete-photo": false,
		"set-presence": false,
	}
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

// ---------------------------------------------------------------------------
// Flag-registration tests for the 7 new verbs
// ---------------------------------------------------------------------------

func TestUserByEmail_FlagsRegistered(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"by-email"})
	if err != nil {
		t.Fatalf("find by-email: %v", err)
	}
	for _, name := range []string{"email"} {
		if sub.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s on user by-email", name)
		}
	}
}

func TestUserPresence_FlagsRegistered(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"presence"})
	if err != nil {
		t.Fatalf("find presence: %v", err)
	}
	if sub.Flags().Lookup("user") == nil {
		t.Error("missing --user on user presence")
	}
}

func TestUserChannels_FlagsRegistered(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"channels"})
	if err != nil {
		t.Fatalf("find channels: %v", err)
	}
	for _, name := range []string{"user", "types"} {
		if sub.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s on user channels", name)
		}
	}
}

func TestUserSetProfile_FlagsRegistered(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"set-profile"})
	if err != nil {
		t.Fatalf("find set-profile: %v", err)
	}
	for _, name := range []string{"name", "value", "profile"} {
		if sub.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s on user set-profile", name)
		}
	}
}

func TestUserSetPhoto_FlagsRegistered(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"set-photo"})
	if err != nil {
		t.Fatalf("find set-photo: %v", err)
	}
	if sub.Flags().Lookup("file") == nil {
		t.Error("missing --file on user set-photo")
	}
}

func TestUserDeletePhoto_FlagsRegistered(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"delete-photo"})
	if err != nil {
		t.Fatalf("find delete-photo: %v", err)
	}
	// delete-photo has no flags; just verify the subcommand exists.
	_ = sub
}

func TestUserSetPresence_FlagsRegistered(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"set-presence"})
	if err != nil {
		t.Fatalf("find set-presence: %v", err)
	}
	if sub.Flags().Lookup("presence") == nil {
		t.Error("missing --presence on user set-presence")
	}
}

// ---------------------------------------------------------------------------
// Dry-run tests for every WRITE verb
// ---------------------------------------------------------------------------

func TestUserSetProfile_DryRun(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{
			name:    "name+value mode",
			args:    []string{"--name", "title", "--value", "Engineer"},
			wantOut: "[dry-run] users.profile.set",
		},
		{
			name:    "profile JSON mode",
			args:    []string{"--profile", `{"title":"Engineer"}`},
			wantOut: "[dry-run] users.profile.set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GlobalFlags{DryRun: true}
			cmd := newUserSetProfileCommand(g)
			var buf strings.Builder
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if !strings.Contains(buf.String(), tt.wantOut) {
				t.Errorf("want %q in output, got %q", tt.wantOut, buf.String())
			}
		})
	}
}

func TestUserSetPhoto_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newUserSetPhotoCommand(g)
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--file", "/tmp/photo.jpg"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "[dry-run] users.setPhoto") || !strings.Contains(got, "/tmp/photo.jpg") {
		t.Errorf("unexpected dry-run output: %q", got)
	}
}

func TestUserDeletePhoto_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newUserDeletePhotoCommand(g)
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run] users.deletePhoto") {
		t.Errorf("unexpected dry-run output: %q", buf.String())
	}
}

func TestUserSetPresence_DryRun(t *testing.T) {
	tests := []struct{ presence string }{{"auto"}, {"away"}}
	for _, tt := range tests {
		t.Run(tt.presence, func(t *testing.T) {
			g := &GlobalFlags{DryRun: true}
			cmd := newUserSetPresenceCommand(g)
			var buf strings.Builder
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs([]string{"--presence", tt.presence})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			got := buf.String()
			if !strings.Contains(got, "[dry-run] users.setPresence") || !strings.Contains(got, tt.presence) {
				t.Errorf("unexpected dry-run output: %q", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// httptest parse tests for READ verbs
// ---------------------------------------------------------------------------

func TestParseUserByEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"user": map[string]any{
				"id":        "U0123456789",
				"name":      "alice",
				"real_name": "Alice",
			},
		})
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("users.lookupByEmail", map[string]string{"email": "alice@example.com"}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	hit, err := parseUserByEmail(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if hit.ID != "U0123456789" || hit.Name != "alice" || hit.Extra != "Alice" {
		t.Errorf("unexpected hit: %+v", hit)
	}
}

func TestUserPresence_Concise(t *testing.T) {
	p := userPresence{Presence: "active", Online: true}
	got := p.Concise()
	if !strings.Contains(got, "presence=active") || !strings.Contains(got, "online=true") {
		t.Errorf("unexpected concise: %q", got)
	}
}

func TestUserPresence_Parse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":       true,
			"presence": "away",
			"online":   false,
		})
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("users.getPresence", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var resp userPresence
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Presence != "away" || resp.Online != false {
		t.Errorf("unexpected resp: %+v", resp)
	}
}

func TestParseUserChannels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"channels": []map[string]any{
				{"id": "C0123456789", "name": "general"},
				{"id": "C0000000001", "name": "random"},
			},
			"response_metadata": map[string]any{"next_cursor": ""},
		})
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	pages, err := c.CallAll("users.conversations", map[string]string{"user": "U0123456789"}, 10)
	if err != nil {
		t.Fatalf("callAll: %v", err)
	}
	var hits []searchHit
	for _, page := range pages {
		pageHits, err := parseUserChannels(page)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		hits = append(hits, pageHits...)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(hits))
	}
	if hits[0].ID != "C0123456789" || hits[0].Name != "general" {
		t.Errorf("unexpected first hit: %+v", hits[0])
	}
	if hits[1].ID != "C0000000001" || hits[1].Name != "random" {
		t.Errorf("unexpected second hit: %+v", hits[1])
	}
}
