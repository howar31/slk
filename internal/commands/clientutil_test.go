package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/howar31/slk/internal/auth"
	"github.com/spf13/cobra"
)

func TestBuildClient_BotGuardrail(t *testing.T) {
	t.Setenv("SLK_CONFIG", filepath.Join(t.TempDir(), "config.toml")) // hermetic; no real config
	t.Setenv("SLK_TOKEN", "xoxb-bot-token")                           // bot token via env
	cmd := &cobra.Command{Use: "messages", Annotations: map[string]string{"botCapable": "false"}}
	_, err := buildClient(cmd, &GlobalFlags{})
	if err == nil {
		t.Fatal("expected guardrail error for user-only verb under a bot token")
	}
	if _, ok := err.(*auth.AuthError); !ok {
		t.Fatalf("expected *auth.AuthError (exit 3), got %T", err)
	}
}

func TestBuildClient_BotAllowedWhenCapable(t *testing.T) {
	t.Setenv("SLK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("SLK_TOKEN", "xoxb-bot-token")
	cmd := &cobra.Command{Use: "send", Annotations: map[string]string{"botCapable": "true"}}
	if _, err := buildClient(cmd, &GlobalFlags{}); err != nil {
		t.Fatalf("unexpected error for bot-capable verb: %v", err)
	}
}

func TestProfileName_FlagBeatsEnv(t *testing.T) {
	t.Setenv("SLK_PROFILE", "from-env")
	got := profileName(&GlobalFlags{Profile: "from-flag"})
	if got != "from-flag" {
		t.Fatalf("profileName: want %q, got %q", "from-flag", got)
	}
}

func TestProfileName_EnvFallback(t *testing.T) {
	t.Setenv("SLK_PROFILE", "from-env")
	got := profileName(&GlobalFlags{})
	if got != "from-env" {
		t.Fatalf("profileName: want %q, got %q", "from-env", got)
	}
}

func TestProfileName_EmptyWhenNothingSet(t *testing.T) {
	t.Setenv("SLK_PROFILE", "")
	got := profileName(&GlobalFlags{})
	if got != "" {
		t.Fatalf("profileName: want empty, got %q", got)
	}
}

func TestCacheDir_UnderConfigSlk(t *testing.T) {
	got := cacheDir()
	want := filepath.Join(".config", "slk", "cache")
	if !strings.HasSuffix(got, want) && !strings.Contains(got, "TemporaryItems") {
		t.Fatalf("cacheDir: want suffix %q (or temp fallback), got %q", want, got)
	}
}
