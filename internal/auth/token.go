package auth

import "fmt"

// ResolveToken picks the active token. Precedence: envToken, then the named
// profile (or cfg.Active if profileName is empty). assertScope, when non-empty
// ("user"|"bot"), requires the resolved token's derived scope to match; a known
// mismatch is an *AuthError. An unknown prefix does not fail the assertion.
func ResolveToken(cfg *Config, profileName, assertScope, envToken string) (string, error) {
	if envToken != "" {
		return assertOK(envToken, assertScope, "SLK_TOKEN")
	}
	name := profileName
	if name == "" {
		name = cfg.Active
	}
	if name == "" {
		return "", &AuthError{Reason: "no profile selected; run 'slk auth set-token' or set SLK_TOKEN"}
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return "", &AuthError{Reason: fmt.Sprintf("profile %q not found", name)}
	}
	if p.Token == "" {
		return "", &AuthError{Reason: fmt.Sprintf("profile %q has no token", name)}
	}
	// A still-encrypted value means Load could not decrypt it (key unavailable).
	if isEncrypted(p.Token) {
		return "", &AuthError{Reason: fmt.Sprintf("profile %q token could not be decrypted (encryption key unavailable)", name)}
	}
	return assertOK(p.Token, assertScope, fmt.Sprintf("profile %q", name))
}

// assertOK returns tok unless assertScope is set and the token's known scope
// differs from it.
func assertOK(tok, assertScope, where string) (string, error) {
	if assertScope != "" {
		if s := TokenScope(tok); s != "" && s != assertScope {
			return "", &AuthError{Reason: fmt.Sprintf("%s holds a %s token but --as %s was requested", where, s, assertScope)}
		}
	}
	return tok, nil
}
