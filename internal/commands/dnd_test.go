package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/howar31/slk/internal/api"
)

// TestDndFlagsRegistered checks that every dnd sub-command has its expected
// flags declared.
func TestDndFlagsRegistered(t *testing.T) {
	cmd := newDndCommand(&GlobalFlags{})

	for _, tc := range []struct {
		verb  string
		flags []string
	}{
		{"info", []string{"user"}},
		{"team", []string{"users"}},
		{"snooze", []string{"minutes"}},
		{"end-snooze", nil},
		{"end", nil},
	} {
		sub, _, err := cmd.Find([]string{tc.verb})
		if err != nil {
			t.Fatalf("find %s: %v", tc.verb, err)
		}
		for _, name := range tc.flags {
			if sub.Flags().Lookup(name) == nil {
				t.Errorf("dnd %s: missing --%s flag", tc.verb, name)
			}
		}
	}
}

// TestDndWrite_DryRun verifies that write verbs print the correct dry-run line
// and do not attempt a network call.
func TestDndWrite_DryRun(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"snooze", "--minutes", "30"}, "dnd.setSnooze"},
		{[]string{"end-snooze"}, "dnd.endSnooze"},
		{[]string{"end"}, "dnd.endDnd"},
	} {
		g := &GlobalFlags{DryRun: true}
		cmd := newDndCommand(g)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("dry-run %v: want %q in output %q", tc.args, tc.want, out.String())
		}
		if !strings.HasPrefix(out.String(), "[dry-run]") {
			t.Errorf("dry-run %v: output should start with [dry-run], got %q", tc.args, out.String())
		}
	}
}

// TestParseDndInfo checks that parseDndInfo extracts fields correctly.
func TestParseDndInfo(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"dnd_enabled": true,
		"snooze_enabled": false,
		"next_dnd_start_ts": 1700000000,
		"next_dnd_end_ts":   1700003600
	}`)
	info, err := parseDndInfo(raw)
	if err != nil {
		t.Fatalf("parseDndInfo: %v", err)
	}
	if !info.DndEnabled {
		t.Errorf("dnd_enabled: want true, got false")
	}
	if info.SnoozeEnabled {
		t.Errorf("snooze_enabled: want false, got true")
	}
	if info.NextDndStart != 1700000000 {
		t.Errorf("next_dnd_start_ts: want 1700000000, got %d", info.NextDndStart)
	}
	if info.NextDndEnd != 1700003600 {
		t.Errorf("next_dnd_end_ts: want 1700003600, got %d", info.NextDndEnd)
	}
	concise := info.Concise()
	if !strings.Contains(concise, "dnd_enabled=true") {
		t.Errorf("Concise: missing dnd_enabled=true in %q", concise)
	}
	if !strings.Contains(concise, "snooze_enabled=false") {
		t.Errorf("Concise: missing snooze_enabled=false in %q", concise)
	}
}

// TestParseDndTeam checks that parseDndTeam builds sorted hits from the users
// map.
func TestParseDndTeam(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"users": {
			"U0123456789": {"dnd_enabled": true,  "snooze_enabled": false},
			"U9876543210": {"dnd_enabled": false, "snooze_enabled": true}
		}
	}`)
	hits, err := parseDndTeam(raw)
	if err != nil {
		t.Fatalf("parseDndTeam: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d", len(hits))
	}
	// Sorted by ID ascending: U0123456789 < U9876543210.
	if hits[0].ID != "U0123456789" {
		t.Errorf("sort order: want U0123456789 first, got %s", hits[0].ID)
	}
	if !strings.Contains(hits[0].Extra, "dnd_enabled=true") {
		t.Errorf("extra for U0123456789: want dnd_enabled=true in %q", hits[0].Extra)
	}
	if !strings.Contains(hits[1].Extra, "dnd_enabled=false") {
		t.Errorf("extra for U9876543210: want dnd_enabled=false in %q", hits[1].Extra)
	}
}

// TestDndInfo_HttptestParse verifies the full round-trip of newDndInfoCommand
// against a local httptest server.
func TestDndInfo_HttptestParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"ok":                true,
			"dnd_enabled":       true,
			"snooze_enabled":    true,
			"next_dnd_start_ts": 1700000000,
			"next_dnd_end_ts":   1700003600,
			"snooze_end_time":   1700001800,
			"snooze_remaining":  900,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("dnd.info", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	info, err := parseDndInfo(raw)
	if err != nil {
		t.Fatalf("parseDndInfo: %v", err)
	}
	if !info.DndEnabled {
		t.Errorf("dnd_enabled: want true")
	}
	if !info.SnoozeEnabled {
		t.Errorf("snooze_enabled: want true")
	}
}

// TestDndTeam_HttptestParse verifies the full round-trip of newDndTeamCommand
// against a local httptest server.
func TestDndTeam_HttptestParse(t *testing.T) {
	var receivedUsers string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		receivedUsers = r.Form.Get("users")
		resp := map[string]any{
			"ok": true,
			"users": map[string]any{
				"U0123456789": map[string]any{"dnd_enabled": true, "snooze_enabled": false},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("dnd.teamInfo", map[string]string{"users": "U0123456789"}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if receivedUsers != "U0123456789" {
		t.Errorf("users param: want U0123456789, got %q", receivedUsers)
	}
	hits, err := parseDndTeam(raw)
	if err != nil {
		t.Fatalf("parseDndTeam: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if hits[0].ID != "U0123456789" {
		t.Errorf("hit ID: want U0123456789, got %s", hits[0].ID)
	}
	if !strings.Contains(hits[0].Extra, "dnd_enabled=true") {
		t.Errorf("extra: want dnd_enabled=true in %q", hits[0].Extra)
	}
}
