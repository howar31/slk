package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_CallAll_FollowsCursor(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			w.Write([]byte(`{"ok":true,"items":[1],"response_metadata":{"next_cursor":"abc"}}`))
		} else {
			w.Write([]byte(`{"ok":true,"items":[2],"response_metadata":{"next_cursor":""}}`))
		}
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL

	pages, err := c.CallAll("conversations.list", nil, 10)
	if err != nil {
		t.Fatalf("callAll: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	var p2 map[string]any
	json.Unmarshal(pages[1], &p2)
	if p2["ok"] != true {
		t.Fatalf("page 2 malformed")
	}
}
