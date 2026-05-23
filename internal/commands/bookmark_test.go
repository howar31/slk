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

func TestBookmarkCommand_FlagsRegistered(t *testing.T) {
	cmd := newBookmarkCommand(&GlobalFlags{})

	for _, tc := range []struct {
		verb  string
		flags []string
	}{
		{"add", []string{"channel", "title", "link", "type"}},
		{"edit", []string{"channel", "id", "title", "link"}},
		{"remove", []string{"channel", "id"}},
		{"list", []string{"channel"}},
	} {
		sub, _, err := cmd.Find([]string{tc.verb})
		if err != nil {
			t.Fatalf("find bookmark %s: %v", tc.verb, err)
		}
		for _, flag := range tc.flags {
			if sub.Flags().Lookup(flag) == nil {
				t.Errorf("missing --%s on bookmark %s", flag, tc.verb)
			}
		}
	}
}

func TestBookmarkWrite_DryRun(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{
			[]string{"add", "--channel", "C0123456789", "--title", "Alice Doc", "--link", "https://example.com"},
			"bookmarks.add",
		},
		{
			[]string{"edit", "--channel", "C0123456789", "--id", "Bk0123456789", "--title", "New Title"},
			"bookmarks.edit",
		},
		{
			[]string{"remove", "--channel", "C0123456789", "--id", "Bk0123456789"},
			"bookmarks.remove",
		},
	} {
		g := &GlobalFlags{DryRun: true}
		cmd := newBookmarkCommand(g)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("want %q in %q", tc.want, out.String())
		}
	}
}

func TestBookmarkEdit_DryRun_OptionalParams(t *testing.T) {
	// Verify that only flags explicitly set are included in dry-run output.
	g := &GlobalFlags{DryRun: true}
	cmd := newBookmarkCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	// Only --title set; --link is absent.
	cmd.SetArgs([]string{"edit", "--channel", "C0123456789", "--id", "Bk0123456789", "--title", "Bob Title"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "title") {
		t.Errorf("expected title in output, got: %q", got)
	}
	// link was not passed; confirm it is absent from params.
	if strings.Contains(got, `"link"`) || strings.Contains(got, `link:`) {
		t.Errorf("link should be absent from dry-run output when not set, got: %q", got)
	}
}

func TestParseBookmarks(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"bookmarks": [
			{"id": "Bk0123456789", "title": "Alice Doc", "link": "https://example.com/alice"},
			{"id": "Bk0000000001", "title": "Bob Repo", "link": "https://example.com/bob"}
		]
	}`)
	hits, err := parseBookmarks(raw)
	if err != nil {
		t.Fatalf("parseBookmarks: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].ID != "Bk0123456789" {
		t.Errorf("hits[0].ID: got %q, want Bk0123456789", hits[0].ID)
	}
	if hits[0].Name != "Alice Doc" {
		t.Errorf("hits[0].Name: got %q, want Alice Doc", hits[0].Name)
	}
	if hits[0].Extra != "https://example.com/alice" {
		t.Errorf("hits[0].Extra: got %q, want https://example.com/alice", hits[0].Extra)
	}
	if hits[1].Name != "Bob Repo" {
		t.Errorf("hits[1].Name: got %q, want Bob Repo", hits[1].Name)
	}
}

func TestBookmarkList_HTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		// Verify the channel_id param was forwarded correctly.
		channelID := r.Form.Get("channel_id")
		if channelID != "C0123456789" {
			http.Error(w, "bad channel_id", http.StatusBadRequest)
			return
		}
		resp := map[string]any{
			"ok": true,
			"bookmarks": []map[string]any{
				{"id": "Bk0123456789", "title": "Alice Guide", "link": "https://example.com/guide"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("bookmarks.list", map[string]string{"channel_id": "C0123456789"}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	hits, err := parseBookmarks(raw)
	if err != nil {
		t.Fatalf("parseBookmarks: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Name != "Alice Guide" {
		t.Errorf("Name: got %q, want Alice Guide", hits[0].Name)
	}
	if hits[0].ID != "Bk0123456789" {
		t.Errorf("ID: got %q, want Bk0123456789", hits[0].ID)
	}
	if hits[0].Extra != "https://example.com/guide" {
		t.Errorf("Extra: got %q, want https://example.com/guide", hits[0].Extra)
	}
}
