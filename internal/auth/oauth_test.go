package auth

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitForCode(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	resultCh := make(chan struct {
		code string
		err  error
	}, 1)
	go func() {
		code, err := WaitForCode(addr, "/callback")
		resultCh <- struct {
			code string
			err  error
		}{code, err}
	}()

	time.Sleep(100 * time.Millisecond)
	_, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?code=the-code", port))
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}

	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("WaitForCode error: %v", res.err)
		}
		if res.code != "the-code" {
			t.Fatalf("got code %q, want %q", res.code, "the-code")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForCode timed out in test")
	}
}

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
