package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestGlobalFlags_Defaults(t *testing.T) {
	g := &GlobalFlags{}
	root := &cobra.Command{Use: "slk", SilenceUsage: true, SilenceErrors: true}
	bindGlobalFlags(root, g)
	root.SetArgs([]string{"--help"})
	_ = root.Execute()
	if g.Format != "concise" {
		t.Fatalf("default format = %q, want concise", g.Format)
	}
	if g.Identity != "" {
		t.Fatalf("default identity = %q, want empty (assertion opt-in)", g.Identity)
	}
}
