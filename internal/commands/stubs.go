package commands

import "github.com/spf13/cobra"

func newAuthCommand(*GlobalFlags) *cobra.Command    { return &cobra.Command{Use: "auth"} }
func newCanvasCommand(*GlobalFlags) *cobra.Command  { return &cobra.Command{Use: "canvas"} }
func newListCommand(*GlobalFlags) *cobra.Command    { return &cobra.Command{Use: "list"} }
func newChannelCommand(*GlobalFlags) *cobra.Command { return &cobra.Command{Use: "channel"} }
