package commands

import (
	"path/filepath"
	"strings"
	"testing"
)

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
