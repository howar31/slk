package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateManifest_WritesUserAndBot(t *testing.T) {
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	content := "intro\n<!-- BEGIN GENERATED MANIFEST -->\nOLD\n<!-- END GENERATED MANIFEST -->\nend\n"
	if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand("test")
	root.SetArgs([]string{"generate-manifest", "--readme", readme})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got, _ := os.ReadFile(readme)
	s := string(got)
	if !strings.Contains(s, `"user"`) || !strings.Contains(s, `"bot"`) {
		t.Fatalf("manifest missing user/bot arrays:\n%s", s)
	}
	if !strings.Contains(s, "channels:manage") || !strings.Contains(s, "users:read.email") {
		t.Fatalf("manifest missing generated scopes:\n%s", s)
	}
	// Slack rejects bot scopes without a bot_user, so the manifest must declare one.
	if !strings.Contains(s, `"bot_user"`) || !strings.Contains(s, `"display_name"`) {
		t.Fatalf("manifest missing features.bot_user:\n%s", s)
	}
	if !strings.HasPrefix(s, "intro\n") || !strings.HasSuffix(s, "end\n") {
		t.Fatalf("content outside markers not preserved:\n%s", s)
	}
}
