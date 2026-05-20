package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_Call_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer xoxp-test" {
			t.Errorf("missing bearer token, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"messages":[{"text":"hi"}]}`))
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("conversations.history", map[string]string{"channel": "C1"}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var got map[string]any
	json.Unmarshal(raw, &got)
	if got["ok"] != true {
		t.Fatalf("expected ok:true, got %v", got)
	}
}

func TestClient_Call_SlackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL

	_, err := c.Call("conversations.history", nil, nil)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.SlackError != "channel_not_found" {
		t.Fatalf("got %q", apiErr.SlackError)
	}
}

func TestClient_Call_RateLimitRetry(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("chat.postMessage", nil, nil)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	var got map[string]any
	json.Unmarshal(raw, &got)
	if got["ok"] != true {
		t.Fatalf("expected ok:true, got %v", got)
	}
	if attempt != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempt)
	}
}

func TestClient_Call_JSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/json; charset=utf-8" {
			t.Errorf("expected Content-Type application/json; charset=utf-8, got %q", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL

	body := []byte(`{"channel":"C1","text":"hello"}`)
	raw, err := c.Call("chat.postMessage", nil, body)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var got map[string]any
	json.Unmarshal(raw, &got)
	if got["ok"] != true {
		t.Fatalf("expected ok:true, got %v", got)
	}
}

func TestClient_Call_RateLimitRetriesExhausted(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "0") // 1s default via parseRetryAfter — keep total bounded
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL
	c.MaxRetries = 2 // 3 total attempts (initial + 2 retries)

	start := time.Now()
	_, err := c.Call("conversations.history", map[string]string{"channel": "C1"}, nil)
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.SlackError != "ratelimited" {
		t.Fatalf("expected SlackError=ratelimited, got %q", apiErr.SlackError)
	}
	if got := ExitCodeFor(apiErr.SlackError); got != 5 {
		t.Errorf("ExitCodeFor(ratelimited) = %d, want 5", got)
	}
	if calls != c.MaxRetries+1 {
		t.Errorf("expected %d total HTTP calls, got %d", c.MaxRetries+1, calls)
	}
	// Sanity bound — should not run away into infinite retries.
	if time.Since(start) > 30*time.Second {
		t.Errorf("retry loop took too long: %v", time.Since(start))
	}
}
