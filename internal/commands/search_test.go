package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/howar31/slk/internal/api"
)

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
	want := map[string]bool{
		"messages": false, "channels": false, "users": false,
		"files": false, "all": false,
	}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing search subcommand %q", name)
		}
	}
}

func TestSearchFiles_FlagsRegistered(t *testing.T) {
	cmd := newSearchCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"files"})
	if err != nil {
		t.Fatalf("find files: %v", err)
	}
	if sub.Flags().Lookup("query") == nil {
		t.Fatal("missing --query on search files")
	}
}

func TestSearchAll_FlagsRegistered(t *testing.T) {
	cmd := newSearchCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"all"})
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	if sub.Flags().Lookup("query") == nil {
		t.Fatal("missing --query on search all")
	}
}

func TestParseSearchFiles(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"ok": true,
		"files": map[string]any{
			"matches": []map[string]any{
				{"id": "F01234567", "name": "report.pdf", "filetype": "pdf"},
				{"id": "F09876543", "name": "photo.png", "filetype": "png"},
			},
		},
	})
	hits, err := parseSearchFiles(raw)
	if err != nil {
		t.Fatalf("parseSearchFiles: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	if hits[0].ID != "F01234567" || hits[0].Name != "report.pdf" || hits[0].Extra != "pdf" {
		t.Errorf("hit 0 = %+v", hits[0])
	}
	if hits[1].Extra != "png" {
		t.Errorf("hit 1 filetype = %q, want png", hits[1].Extra)
	}
}

func TestParseSearchAll(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"ok": true,
		"messages": map[string]any{
			"matches": []map[string]any{
				{"username": "Alice", "ts": "1234567890.000001"},
			},
		},
		"files": map[string]any{
			"matches": []map[string]any{
				{"id": "F01234567", "name": "slide.pdf", "filetype": "pdf"},
			},
		},
	})
	hits, err := parseSearchAll(raw)
	if err != nil {
		t.Fatalf("parseSearchAll: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	// First hit is the message.
	if hits[0].Name != "Alice" || hits[0].ID != "1234567890.000001" || hits[0].Extra != "message" {
		t.Errorf("message hit = %+v", hits[0])
	}
	// Second hit is the file.
	if hits[1].ID != "F01234567" || hits[1].Name != "slide.pdf" || hits[1].Extra != "pdf" {
		t.Errorf("file hit = %+v", hits[1])
	}
}

func TestParseSearchFiles_Via_HTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"files": map[string]any{
				"matches": []map[string]any{
					{"id": "F01234567", "name": "notes.txt", "filetype": "text"},
				},
			},
		})
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("search.files", map[string]string{"query": "notes"}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	hits, err := parseSearchFiles(raw)
	if err != nil {
		t.Fatalf("parseSearchFiles: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "F01234567" {
		t.Fatalf("unexpected hits: %+v", hits)
	}
}

func TestParseSearchAll_Via_HTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"messages": map[string]any{
				"matches": []map[string]any{
					{"username": "Bob", "ts": "9876543210.000002"},
				},
			},
			"files": map[string]any{
				"matches": []map[string]any{
					{"id": "F01234567", "name": "deck.pdf", "filetype": "pdf"},
				},
			},
		})
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("search.all", map[string]string{"query": "deck"}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	hits, err := parseSearchAll(raw)
	if err != nil {
		t.Fatalf("parseSearchAll: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	if hits[0].Extra != "message" {
		t.Errorf("first hit should be message, got %+v", hits[0])
	}
	if hits[1].Extra != "pdf" {
		t.Errorf("second hit should be pdf, got %+v", hits[1])
	}
}
