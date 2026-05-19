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
		return "", fmt.Errorf("no profile selected; run 'slk auth set-token' or set SLK_TOKEN")
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return "", fmt.Errorf("profile %q not found", name)
	}
	switch identity {
	case "bot":
		if p.BotToken == "" {
			return "", fmt.Errorf("profile %q has no bot token", name)
		}
		return p.BotToken, nil
	default:
		if p.UserToken == "" {
			return "", fmt.Errorf("profile %q has no user token", name)
		}
		return p.UserToken, nil
	}
}
