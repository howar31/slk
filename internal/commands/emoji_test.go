package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/howar31/slk/internal/api"
)

func TestEmojiCommand_HasSubcommands(t *testing.T) {
	cmd := newEmojiCommand(&GlobalFlags{})
	want := map[string]bool{"list": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing emoji subcommand %q", name)
		}
	}
}

func TestParseEmojiList_SortedAndAliasKept(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"emoji": map[string]any{
				"wave":      "https://emoji.slack-edge.com/T0123456789/wave/abc.png",
				"celebrate": "alias:tada",
				"boom":      "https://emoji.slack-edge.com/T0123456789/boom/def.png",
			},
		})
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("emoji.list", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	hits, err := parseEmojiList(raw)
	if err != nil {
		t.Fatalf("parseEmojiList: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("expected 3 emoji, got %d", len(hits))
	}
	// Verify sorted order: boom, celebrate, wave.
	if hits[0].Name != "boom" {
		t.Errorf("hits[0].Name = %q, want boom", hits[0].Name)
	}
	if hits[1].Name != "celebrate" {
		t.Errorf("hits[1].Name = %q, want celebrate", hits[1].Name)
	}
	if hits[2].Name != "wave" {
		t.Errorf("hits[2].Name = %q, want wave", hits[2].Name)
	}
	// Alias entry must be preserved with its alias: prefix.
	if hits[1].Extra != "alias:tada" {
		t.Errorf("alias Extra = %q, want alias:tada", hits[1].Extra)
	}
	// ID field is empty for emoji (no Slack ID in the response).
	for _, h := range hits {
		if h.ID != "" {
			t.Errorf("emoji hit should have empty ID, got %q for %q", h.ID, h.Name)
		}
	}
}

func TestParseEmojiList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    true,
			"emoji": map[string]any{},
		})
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("emoji.list", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	hits, err := parseEmojiList(raw)
	if err != nil {
		t.Fatalf("parseEmojiList empty: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits for empty emoji map, got %d", len(hits))
	}
}
