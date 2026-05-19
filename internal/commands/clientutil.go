package commands

import (
	"os"
	"path/filepath"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/auth"
)

// profileName returns the effective profile: the --profile flag if set,
// otherwise the SLK_PROFILE env var.
func profileName(g *GlobalFlags) string {
	if g.Profile != "" {
		return g.Profile
	}
	return os.Getenv("SLK_PROFILE")
}

// buildClient resolves the active token and returns a ready API client.
func buildClient(g *GlobalFlags) (*api.Client, error) {
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
