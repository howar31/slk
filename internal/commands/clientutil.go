package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/auth"
	"github.com/spf13/cobra"
)

// profileName returns the effective profile: the --profile flag if set,
// otherwise the SLK_PROFILE env var.
func profileName(g *GlobalFlags) string {
	if g.Profile != "" {
		return g.Profile
	}
	return os.Getenv("SLK_PROFILE")
}

// buildClient resolves the active token (asserting --as scope) and returns a
// ready API client. It refuses a user-only verb (botCapable=false) when the
// resolved token is a bot token, failing fast with an *auth.AuthError (exit 3)
// instead of surfacing Slack's not_allowed_token_type.
func buildClient(cmd *cobra.Command, g *GlobalFlags) (*api.Client, error) {
	path, err := auth.ConfigPath()
	if err != nil {
		return nil, err
	}
	cfg, err := auth.Load(path)
	if err != nil {
		return nil, err
	}
	token, err := auth.ResolveToken(cfg, profileName(g), g.Identity, os.Getenv("SLK_TOKEN"))
	if err != nil {
		return nil, err
	}
	if auth.TokenScope(token) == "bot" && cmd.Annotations["botCapable"] == "false" {
		return nil, &auth.AuthError{Reason: fmt.Sprintf("%q is user-token-only; the active profile holds a bot token", cmd.CommandPath())}
	}
	return api.New(token), nil
}

// cacheDir returns ~/.config/slk/cache. Consumed by the ID resolver wired in
// Task 9.2 (newResolver); defined here alongside buildClient as shared command
// infrastructure.
func cacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return filepath.Join(home, ".config", "slk", "cache")
}
