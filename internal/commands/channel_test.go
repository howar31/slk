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
		{[]string{"archive", "--channel", "C1"}, "conversations.archive"},
		{[]string{"invite", "--channel", "C1", "--users", "U1,U2"}, "conversations.invite"},
		{[]string{"topic", "--channel", "C1", "--topic", "hello"}, "conversations.setTopic"},
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
