package commands

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howar31/slk/internal/auth"
)

func TestAuthSetTokenAndStatus(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)

	set := newAuthCommand(&GlobalFlags{})
	set.SetArgs([]string{"set-token", "--profile", "work", "--user", "xoxp-x", "--workspace", "acme"})
	if err := set.Execute(); err != nil {
		t.Fatalf("set-token: %v", err)
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil || cfg.Profiles["work"].UserToken != "xoxp-x" {
		t.Fatalf("token not persisted: %+v err=%v", cfg, err)
	}

	status := newAuthCommand(&GlobalFlags{})
	var out bytes.Buffer
	status.SetOut(&out)
	status.SetArgs([]string{"status"})
	if err := status.Execute(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "work") {
		t.Fatalf("status missing profile: %q", out.String())
	}
	if strings.Contains(out.String(), "xoxp-x") {
		t.Fatalf("status output leaked the raw token: %q", out.String())
	}
}
