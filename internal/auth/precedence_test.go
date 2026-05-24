package auth

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveToken_PrecedenceAndAssertion locks down the precedence model and
// the scope-assertion semantics described in design spec §6:
//  1. envToken (from SLK_TOKEN) always wins
//  2. else: explicit profileName flag overrides cfg.Active
//  3. else: cfg.Active is used
//  4. assertScope, when non-empty, must match the token's derived scope
func TestResolveToken_PrecedenceAndAssertion(t *testing.T) {
	t.Setenv(keyEnvVar, backendFile)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	cfg := &Config{
		Active: "work",
		Profiles: map[string]Profile{
			"work":     {Token: "xoxb-work-bot"},
			"personal": {Token: "xoxp-personal-user"},
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
		assertScope string
		envToken    string
		wantToken   string
		wantErr     bool
	}{
		{"active profile, no assertion", "", "", "", "xoxb-work-bot", false},
		{"active profile, matching assertion", "", "bot", "", "xoxb-work-bot", false},
		{"active profile, mismatched assertion", "", "user", "", "", true},
		{"explicit profile overrides active", "personal", "", "", "xoxp-personal-user", false},
		{"explicit profile, matching assertion", "personal", "user", "", "xoxp-personal-user", false},
		{"env token wins over everything", "personal", "", "xoxp-env", "xoxp-env", false},
		{"env token still asserted", "personal", "bot", "xoxp-env", "", true},
		{"missing profile is an error", "ghost", "", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveToken(loaded, tc.profileName, tc.assertScope, tc.envToken)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.wantToken {
				t.Errorf("token = %q, want %q", got, tc.wantToken)
			}
		})
	}

	// AuthError type is preserved for the assertion-mismatch case so callers
	// get exit code 3 instead of 1.
	_, err = ResolveToken(loaded, "", "user", "")
	if _, ok := err.(*AuthError); !ok {
		t.Errorf("expected *AuthError for assertion mismatch, got %T", err)
	}

	// SLK_CONFIG env override path resolves.
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
			"work": {Token: encPrefix + "bm90LXJlYWwtY2lwaGVydGV4dA=="},
		},
	}
	_, err := ResolveToken(cfg, "", "", "")
	if err == nil {
		t.Fatal("expected an error for an undecryptable token")
	}
	if _, ok := err.(*AuthError); !ok {
		t.Fatalf("expected *AuthError (exit 3), got %T", err)
	}

	// SLK_TOKEN must still bypass everything, even with a broken stored token.
	tok, err := ResolveToken(cfg, "", "", "xoxp-env")
	if err != nil {
		t.Fatalf("env override should bypass the broken token: %v", err)
	}
	if tok != "xoxp-env" {
		t.Fatalf("env token = %q, want xoxp-env", tok)
	}
}
