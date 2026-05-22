// Package commands defines the slk command tree.
package commands

import "github.com/spf13/cobra"

// NewRootCommand builds the slk root command for the given build version.
func NewRootCommand(version string) *cobra.Command {
	g := &GlobalFlags{}
	root := &cobra.Command{
		Use:           "slk",
		Short:         "Agent-facing Slack CLI",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Keep `slk --version` and the `version` subcommand byte-for-byte
	// consistent ("slk <version>"); Cobra's default template prints
	// "slk version <version>".
	root.SetVersionTemplate("slk {{.Version}}\n")
	bindGlobalFlags(root, g)
	root.AddCommand(
		newAPICommand(g),
		newAuthCommand(g),
		newMsgCommand(g),
		newThreadCommand(g),
		newSearchCommand(g),
		newCanvasCommand(g),
		newListCommand(g),
		newChannelCommand(g),
		newUserCommand(g),
		newVersionCommand(g, version),
	)
	return root
}
