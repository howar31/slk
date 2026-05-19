package commands

import "github.com/spf13/cobra"

func newAuthCommand(*GlobalFlags) *cobra.Command { return &cobra.Command{Use: "auth"} }
