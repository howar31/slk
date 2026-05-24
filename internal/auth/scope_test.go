package auth

import "testing"

func TestTokenScope(t *testing.T) {
	cases := map[string]string{
		"xoxp-abc":   "user",
		"xoxb-abc":   "bot",
		"xoxc-abc":   "",
		"xapp-abc":   "",
		"":           "",
		"enc:v1:zzz": "",
	}
	for tok, want := range cases {
		if got := TokenScope(tok); got != want {
			t.Errorf("TokenScope(%q) = %q, want %q", tok, got, want)
		}
	}
}
