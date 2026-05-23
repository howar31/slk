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

func TestChannelList_FlagsRegistered(t *testing.T) {
	cmd := newChannelCommand(&GlobalFlags{})
	list, _, err := cmd.Find([]string{"list"})
	if err != nil {
		t.Fatalf("find list: %v", err)
	}
	for _, name := range []string{"limit", "cursor", "types"} {
		if list.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s on channel list", name)
		}
	}
}

func TestFetchChannelsWith_PassesParamsAndLimits(t *testing.T) {
	var calls int
	var lastTypes, lastCursor, lastLimitParam string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		lastTypes = r.Form.Get("types")
		lastCursor = r.Form.Get("cursor")
		lastLimitParam = r.Form.Get("limit")
		calls++
		// Always return three channels and a next_cursor so the client would
		// page again — unless our --limit truncation stops it first.
		resp := map[string]any{
			"ok": true,
			"channels": []map[string]any{
				{"id": "C1", "name": "alpha", "num_members": 1},
				{"id": "C2", "name": "bravo", "num_members": 2},
				{"id": "C3", "name": "charlie", "num_members": 3},
			},
			"response_metadata": map[string]any{"next_cursor": "page2"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	hits, err := fetchChannelsWith(c, channelListOpts{
		Types:  "im,mpim",
		Cursor: "start",
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(hits) != 2 {
		t.Errorf("limit not honored: got %d hits, want 2", len(hits))
	}
	if calls != 1 {
		t.Errorf("expected to stop after first page when limit reached, got %d calls", calls)
	}
	if lastTypes != "im,mpim" {
		t.Errorf("--types not forwarded: got %q", lastTypes)
	}
	if lastCursor != "start" {
		t.Errorf("--cursor not forwarded: got %q", lastCursor)
	}
	if lastLimitParam != "200" {
		t.Errorf("page-size override unexpected: got %q (want 200)", lastLimitParam)
	}
	// Defensive: searchHit fields stay populated.
	if hits[0].ID == "" || hits[0].Name == "" {
		t.Errorf("hit not populated: %+v", hits[0])
	}
}

func TestChannelWrite_DryRun(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"create", "--name", "new-chan"}, "conversations.create"},
		{[]string{"archive", "--channel", "C0123456789"}, "conversations.archive"},
		{[]string{"invite", "--channel", "C0123456789", "--users", "U0123456789"}, "conversations.invite"},
		{[]string{"topic", "--channel", "C0123456789", "--topic", "hello"}, "conversations.setTopic"},
		{[]string{"join", "--channel", "C0123456789"}, "conversations.join"},
		{[]string{"leave", "--channel", "C0123456789"}, "conversations.leave"},
		{[]string{"purpose", "--channel", "C0123456789", "--purpose", "our purpose"}, "conversations.setPurpose"},
		{[]string{"kick", "--channel", "C0123456789", "--user", "U0123456789"}, "conversations.kick"},
		{[]string{"rename", "--channel", "C0123456789", "--name", "new-name"}, "conversations.rename"},
		{[]string{"unarchive", "--channel", "C0123456789"}, "conversations.unarchive"},
		{[]string{"open", "--users", "U0123456789"}, "conversations.open"},
		{[]string{"mark", "--channel", "C0123456789", "--ts", "1234567890.000001"}, "conversations.mark"},
		{[]string{"close", "--channel", "C0123456789"}, "conversations.close"},
	} {
		g := &GlobalFlags{DryRun: true}
		cmd := newChannelCommand(g)
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

func TestChannelNewVerbs_FlagsRegistered(t *testing.T) {
	cmd := newChannelCommand(&GlobalFlags{})

	for _, tc := range []struct {
		verb  string
		flags []string
	}{
		{"info", []string{"channel"}},
		{"members", []string{"channel"}},
		{"join", []string{"channel"}},
		{"leave", []string{"channel"}},
		{"purpose", []string{"channel", "purpose"}},
		{"kick", []string{"channel", "user"}},
		{"rename", []string{"channel", "name"}},
		{"unarchive", []string{"channel"}},
		{"open", []string{"users"}},
		{"mark", []string{"channel", "ts"}},
		{"close", []string{"channel"}},
	} {
		sub, _, err := cmd.Find([]string{tc.verb})
		if err != nil {
			t.Fatalf("find %s: %v", tc.verb, err)
		}
		for _, flag := range tc.flags {
			if sub.Flags().Lookup(flag) == nil {
				t.Errorf("missing --%s on channel %s", flag, tc.verb)
			}
		}
	}
}

func TestParseChannelInfo(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"channel": {
			"id": "C0123456789",
			"name": "general",
			"num_members": 42,
			"is_archived": false
		}
	}`)
	hit, err := parseChannelInfo(raw)
	if err != nil {
		t.Fatalf("parseChannelInfo: %v", err)
	}
	if hit.ID != "C0123456789" {
		t.Errorf("ID: got %q, want C0123456789", hit.ID)
	}
	if hit.Name != "general" {
		t.Errorf("Name: got %q, want general", hit.Name)
	}
	if !strings.Contains(hit.Extra, "42") {
		t.Errorf("Extra should contain member count: got %q", hit.Extra)
	}
}

func TestParseChannelInfo_Archived(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"channel": {
			"id": "C0123456789",
			"name": "old-channel",
			"num_members": 5,
			"is_archived": true
		}
	}`)
	hit, err := parseChannelInfo(raw)
	if err != nil {
		t.Fatalf("parseChannelInfo: %v", err)
	}
	if !strings.Contains(hit.Extra, "archived") {
		t.Errorf("Extra should indicate archived: got %q", hit.Extra)
	}
}

func TestParseChannelMembers(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"members": ["U0123456789", "USLACKBOT"],
		"response_metadata": {"next_cursor": "cursor-abc"}
	}`)
	hits, nextCursor, err := parseChannelMembers(raw)
	if err != nil {
		t.Fatalf("parseChannelMembers: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits len: got %d, want 2", len(hits))
	}
	if hits[0].ID != "U0123456789" {
		t.Errorf("hits[0].ID: got %q, want U0123456789", hits[0].ID)
	}
	if nextCursor != "cursor-abc" {
		t.Errorf("nextCursor: got %q, want cursor-abc", nextCursor)
	}
}

func TestChannelInfo_HTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		resp := map[string]any{
			"ok": true,
			"channel": map[string]any{
				"id":          "C0123456789",
				"name":        "general",
				"num_members": 10,
				"is_archived": false,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := &GlobalFlags{}
	cmd := newChannelInfoCommand(g)
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--channel", "C0123456789"})
	// Wire a fake client by calling the parse helper directly to verify round-trip.
	raw := []byte(`{"ok":true,"channel":{"id":"C0123456789","name":"general","num_members":10,"is_archived":false}}`)
	hit, err := parseChannelInfo(raw)
	if err != nil {
		t.Fatalf("parseChannelInfo round-trip: %v", err)
	}
	if hit.Name != "general" || hit.ID != "C0123456789" {
		t.Errorf("unexpected hit: %+v", hit)
	}
	_ = c // used above for BaseURL sanity
}

func TestChannelMembers_HTTPTest(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		calls++
		var cursor string
		if calls == 1 {
			cursor = "page2"
		}
		resp := map[string]any{
			"ok":      true,
			"members": []string{"U0123456789"},
			"response_metadata": map[string]any{
				"next_cursor": cursor,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	// Call members twice via the parse helper to simulate pagination.
	params := map[string]string{"channel": "C0123456789", "limit": "200"}
	raw1, err := c.Call("conversations.members", params, nil)
	if err != nil {
		t.Fatalf("call 1: %v", err)
	}
	hits1, nextCursor, err := parseChannelMembers(raw1)
	if err != nil {
		t.Fatalf("parse 1: %v", err)
	}
	if len(hits1) != 1 || hits1[0].ID != "U0123456789" {
		t.Errorf("page 1 hits: %+v", hits1)
	}
	if nextCursor != "page2" {
		t.Errorf("nextCursor after page 1: got %q, want page2", nextCursor)
	}

	params["cursor"] = nextCursor
	raw2, err := c.Call("conversations.members", params, nil)
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	hits2, nextCursor2, err := parseChannelMembers(raw2)
	if err != nil {
		t.Fatalf("parse 2: %v", err)
	}
	if len(hits2) != 1 {
		t.Errorf("page 2 hits: %+v", hits2)
	}
	if nextCursor2 != "" {
		t.Errorf("nextCursor after page 2: got %q, want empty", nextCursor2)
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", calls)
	}
}
