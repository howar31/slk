package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") != "the-code" {
			t.Errorf("missing code, got %q", r.Form.Get("code"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ok":           true,
			"access_token": "xoxb-bot",
			"authed_user":  map[string]string{"access_token": "xoxp-user"},
		})
	}))
	defer srv.Close()

	got, err := ExchangeCode(srv.URL, "cid", "secret", "the-code", "http://localhost:0/cb")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if got.UserToken != "xoxp-user" || got.BotToken != "xoxb-bot" {
		t.Fatalf("unexpected tokens: %+v", got)
	}
}
