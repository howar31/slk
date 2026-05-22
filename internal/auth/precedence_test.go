package auth

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveToken_PrecedenceMatrix locks down the four-axis precedence model
// described in design spec §6:
//  1. envToken (from SLK_TOKEN) always wins
//  2. else: explicit profileName flag overrides cfg.Active
//  3. else: cfg.Active is used
//  4. identity (user|bot) selects the right token within a profile
func TestResolveToken_PrecedenceMatrix(t *testing.T) {
	t.Setenv(keyEnvVar, backendFile)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	// Write a real config with two profiles, each having both tokens.
	cfg := &Config{
		Active: "work",
		Profiles: map[string]Profile{
			"work":     {Workspace: "acme", UserToken: "xoxp-work-user", BotToken: "xoxb-work-bot"},
			"personal": {Workspace: "private", UserToken: "xoxp-personal-user", BotToken: "xoxb-personal-bot"},
		},
	}
	if err := Save(cfgPath, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for _, tc := range []struct {
		name        string
		profileName string
		identity    string
		envToken    string
		wantToken   string
		wantErr     bool
	}{
		{"active profile, default identity", "", "user", "", "xoxp-work-user", false},
		{"active profile, bot identity", "", "bot", "", "xoxb-work-bot", false},
		{"explicit profile flag overrides active", "personal", "user", "", "xoxp-personal-user", false},
		{"personal + bot", "personal", "bot", "", "xoxb-personal-bot", false},
		{"env token wins over everything", "personal", "user", "xoxp-env-override", "xoxp-env-override", false},
		{"env token wins even with bot identity", "personal", "bot", "xoxp-env-override", "xoxp-env-override", false},
		{"missing profile is an error", "ghost", "user", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveToken(loaded, tc.profileName, tc.identity, tc.envToken)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.wantToken {
				t.Errorf("token = %q, want %q", got, tc.wantToken)
			}
		})
	}

	// Bonus: AuthError type is preserved for the missing-profile case so
	// callers get exit code 3 instead of 1.
	_, err = ResolveToken(loaded, "ghost", "user", "")
	if _, ok := err.(*AuthError); !ok {
		t.Errorf("expected *AuthError for missing profile, got %T", err)
	}

	// Bonus: SLK_CONFIG env override path resolves.
	t.Setenv("SLK_CONFIG", cfgPath)
	got, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != cfgPath {
		t.Errorf("ConfigPath = %q, want %q", got, cfgPath)
	}
	os.Unsetenv("SLK_CONFIG") // explicit; t.Setenv also cleans up
}

func TestResolveToken_UndecryptableTokenErrors(t *testing.T) {
	// A profile whose token is ciphertext that no key can open (decryption was
	// left to fail at Load) must surface as an *AuthError, not a leaked blob.
	cfg := &Config{
		Active: "work",
		Profiles: map[string]Profile{
			"work": {UserToken: encPrefix + "bm90LXJlYWwtY2lwaGVydGV4dA=="},
		},
	}
	_, err := ResolveToken(cfg, "", "user", "")
	if err == nil {
		t.Fatal("expected an error for an undecryptable token")
	}
	if _, ok := err.(*AuthError); !ok {
		t.Fatalf("expected *AuthError (exit 3), got %T", err)
	}

	// SLK_TOKEN must still bypass everything, even with a broken stored token.
	tok, err := ResolveToken(cfg, "", "user", "xoxp-env")
	if err != nil {
		t.Fatalf("env override should bypass the broken token: %v", err)
	}
	if tok != "xoxp-env" {
		t.Fatalf("env token = %q, want xoxp-env", tok)
	}
}
