package auth

import "testing"

func TestResolveToken_EnvWins(t *testing.T) {
	cfg := &Config{Active: "work", Profiles: map[string]Profile{
		"work": {Token: "xoxb-config"},
	}}
	tok, err := ResolveToken(cfg, "", "", "xoxp-env")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tok != "xoxp-env" {
		t.Fatalf("env should win, got %q", tok)
	}
}

func TestResolveToken_SingleToken(t *testing.T) {
	cfg := &Config{Active: "work", Profiles: map[string]Profile{
		"work": {Token: "xoxb-bot"},
	}}
	tok, err := ResolveToken(cfg, "", "", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tok != "xoxb-bot" {
		t.Fatalf("got %q, want xoxb-bot", tok)
	}
}

func TestResolveToken_MissingProfile(t *testing.T) {
	cfg := &Config{Profiles: map[string]Profile{}}
	if _, err := ResolveToken(cfg, "", "", ""); err == nil {
		t.Fatal("expected error when no profile and no env token")
	}
}
