package auth

import "testing"

func TestResolveToken_EnvWins(t *testing.T) {
	cfg := &Config{Active: "work", Profiles: map[string]Profile{
		"work": {UserToken: "xoxp-config"},
	}}
	tok, err := ResolveToken(cfg, "", "user", "xoxp-env")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tok != "xoxp-env" {
		t.Fatalf("env should win, got %q", tok)
	}
}

func TestResolveToken_IdentitySelection(t *testing.T) {
	cfg := &Config{Active: "work", Profiles: map[string]Profile{
		"work": {UserToken: "xoxp-u", BotToken: "xoxb-b"},
	}}
	u, _ := ResolveToken(cfg, "", "user", "")
	b, _ := ResolveToken(cfg, "", "bot", "")
	if u != "xoxp-u" || b != "xoxb-b" {
		t.Fatalf("identity selection failed: user=%q bot=%q", u, b)
	}
}

func TestResolveToken_MissingProfile(t *testing.T) {
	cfg := &Config{Profiles: map[string]Profile{}}
	if _, err := ResolveToken(cfg, "", "user", ""); err == nil {
		t.Fatal("expected error when no profile and no env token")
	}
}
