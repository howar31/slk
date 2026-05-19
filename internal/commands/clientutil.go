package commands

import (
	"os"
	"path/filepath"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/auth"
)

// buildClient resolves the active token and returns a ready API client.
func buildClient(g *GlobalFlags) (*api.Client, error) {
	path, err := auth.DefaultPath()
	if err != nil {
		return nil, err
	}
	cfg, err := auth.Load(path)
	if err != nil {
		return nil, err
	}
	token, err := auth.ResolveToken(cfg, g.Profile, g.Identity, os.Getenv("SLK_TOKEN"))
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
