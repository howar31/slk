// Package commands defines the slk command tree.
package commands

import "github.com/spf13/cobra"

// NewRootCommand builds the slk root command for the given build version.
func NewRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "slk",
		Short:         "Agent-facing Slack CLI",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return root
}
