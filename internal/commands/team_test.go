package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/howar31/slk/internal/api"
)

func TestTeamCommand_HasSubcommands(t *testing.T) {
	cmd := newTeamCommand(&GlobalFlags{})
	want := map[string]bool{"info": false, "profile": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing team subcommand %q", name)
		}
	}
}

func TestTeamInfo_FlagsRegistered(t *testing.T) {
	cmd := newTeamCommand(&GlobalFlags{})
	info, _, err := cmd.Find([]string{"info"})
	if err != nil {
		t.Fatalf("find info: %v", err)
	}
	if info.Flags().Lookup("team") == nil {
		t.Error("missing --team on team info")
	}
}

func TestParseTeamInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"team": map[string]any{
				"id":     "T0123456789",
				"name":   "Acme Inc",
				"domain": "acme",
			},
		})
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("team.info", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	hit, err := parseTeamInfo(raw)
	if err != nil {
		t.Fatalf("parseTeamInfo: %v", err)
	}
	if hit.ID != "T0123456789" {
		t.Errorf("ID = %q, want T0123456789", hit.ID)
	}
	if hit.Name != "Acme Inc" {
		t.Errorf("Name = %q, want Acme Inc", hit.Name)
	}
	if hit.Extra != "acme.slack.com" {
		t.Errorf("Extra = %q, want acme.slack.com", hit.Extra)
	}
}

func TestParseTeamProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"profile": map[string]any{
				"fields": []map[string]any{
					{"id": "Xf0123456789", "label": "Department", "type": "text"},
					{"id": "Xf9876543210", "label": "Start Date", "type": "date"},
				},
			},
		})
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("team.profile.get", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	hits, err := parseTeamProfile(raw)
	if err != nil {
		t.Fatalf("parseTeamProfile: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(hits))
	}
	if hits[0].ID != "Xf0123456789" || hits[0].Name != "Department" || hits[0].Extra != "text" {
		t.Errorf("field[0] = %+v", hits[0])
	}
	if hits[1].ID != "Xf9876543210" || hits[1].Name != "Start Date" || hits[1].Extra != "date" {
		t.Errorf("field[1] = %+v", hits[1])
	}
}
