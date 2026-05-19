package commands

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/howar31/slk/internal/api"
)

func TestNewResolver_NilWhenNoResolve(t *testing.T) {
	if newResolver(&GlobalFlags{NoResolve: true}, nil) != nil {
		t.Fatal("expected nil resolver when --no-resolve is set")
	}
}

func TestResolveUser_NilResolverPassthrough(t *testing.T) {
	if got := resolveUser(nil, "U123"); got != "U123" {
		t.Fatalf("nil resolver should pass through, got %q", got)
	}
}

func TestSlackLookup_Name(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"user":{"name":"bob","real_name":"Bob Brown"}}`))
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL
	name, err := slackLookup{c}.Name("U1")
	if err != nil || name != "Bob Brown" {
		t.Fatalf("name=%q err=%v", name, err)
	}
}
