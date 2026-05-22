package auth

import "fmt"

// ResolveToken picks the active token. Precedence: envToken, then the named
// profile (or cfg.Active if profileName is empty). identity is "user" or "bot".
func ResolveToken(cfg *Config, profileName, identity, envToken string) (string, error) {
	if envToken != "" {
		return envToken, nil
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

	var tok string
	switch identity {
	case "bot":
		if p.BotToken == "" {
			return "", &AuthError{Reason: fmt.Sprintf("profile %q has no bot token", name)}
		}
		tok = p.BotToken
	default:
		if p.UserToken == "" {
			return "", &AuthError{Reason: fmt.Sprintf("profile %q has no user token", name)}
		}
		tok = p.UserToken
	}

	// A still-encrypted value means Load could not decrypt it (the encryption
	// key was unavailable or wrong). Fail as an auth error rather than handing
	// back ciphertext.
	if isEncrypted(tok) {
		return "", &AuthError{Reason: fmt.Sprintf("profile %q token could not be decrypted (encryption key unavailable)", name)}
	}
	return tok, nil
}
