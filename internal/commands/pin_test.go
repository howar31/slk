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

func TestPinCommand_FlagsRegistered(t *testing.T) {
	cmd := newPinCommand(&GlobalFlags{})

	for _, tc := range []struct {
		verb  string
		flags []string
	}{
		{"add", []string{"channel", "ts"}},
		{"remove", []string{"channel", "ts"}},
		{"list", []string{"channel"}},
	} {
		sub, _, err := cmd.Find([]string{tc.verb})
		if err != nil {
			t.Fatalf("find pin %s: %v", tc.verb, err)
		}
		for _, flag := range tc.flags {
			if sub.Flags().Lookup(flag) == nil {
				t.Errorf("missing --%s on pin %s", flag, tc.verb)
			}
		}
	}
}

func TestPinWrite_DryRun(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{
			[]string{"add", "--channel", "C0123456789", "--ts", "1234567890.000001"},
			"pins.add",
		},
		{
			[]string{"remove", "--channel", "C0123456789", "--ts", "1234567890.000001"},
			"pins.remove",
		},
	} {
		g := &GlobalFlags{DryRun: true}
		cmd := newPinCommand(g)
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

func TestParsePinItems_Message(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"items": [
			{"type": "message", "message": {"ts": "1234567890.000001"}}
		]
	}`)
	hits, err := parsePinItems(raw)
	if err != nil {
		t.Fatalf("parsePinItems: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].ID != "1234567890.000001" {
		t.Errorf("ID: got %q, want 1234567890.000001", hits[0].ID)
	}
	if hits[0].Extra != "message" {
		t.Errorf("Extra: got %q, want message", hits[0].Extra)
	}
}

func TestParsePinItems_File(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"items": [
			{"type": "file", "file": {"id": "F01234567", "name": "report.pdf"}}
		]
	}`)
	hits, err := parsePinItems(raw)
	if err != nil {
		t.Fatalf("parsePinItems: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].ID != "F01234567" {
		t.Errorf("ID: got %q, want F01234567", hits[0].ID)
	}
	if hits[0].Name != "report.pdf" {
		t.Errorf("Name: got %q, want report.pdf", hits[0].Name)
	}
	if hits[0].Extra != "file" {
		t.Errorf("Extra: got %q, want file", hits[0].Extra)
	}
}

func TestParsePinItems_Mixed(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"items": [
			{"type": "message", "message": {"ts": "1234567890.000001"}},
			{"type": "file", "file": {"id": "F01234567", "name": "notes.txt"}}
		]
	}`)
	hits, err := parsePinItems(raw)
	if err != nil {
		t.Fatalf("parsePinItems: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].Extra != "message" {
		t.Errorf("hits[0].Extra: got %q, want message", hits[0].Extra)
	}
	if hits[1].Extra != "file" {
		t.Errorf("hits[1].Extra: got %q, want file", hits[1].Extra)
	}
}

func TestPinList_HTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		resp := map[string]any{
			"ok": true,
			"items": []map[string]any{
				{"type": "message", "message": map[string]any{"ts": "1234567890.000001"}},
				{"type": "file", "file": map[string]any{"id": "F01234567", "name": "Alice-notes.txt"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("pins.list", map[string]string{"channel": "C0123456789"}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	hits, err := parsePinItems(raw)
	if err != nil {
		t.Fatalf("parsePinItems: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].ID != "1234567890.000001" {
		t.Errorf("hits[0].ID: got %q, want 1234567890.000001", hits[0].ID)
	}
	if hits[1].Name != "Alice-notes.txt" {
		t.Errorf("hits[1].Name: got %q, want Alice-notes.txt", hits[1].Name)
	}
}
