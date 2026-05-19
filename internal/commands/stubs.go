package commands

import "github.com/spf13/cobra"

func newAPICommand(*GlobalFlags) *cobra.Command     { return &cobra.Command{Use: "api"} }
func newAuthCommand(*GlobalFlags) *cobra.Command    { return &cobra.Command{Use: "auth"} }
func newMsgCommand(*GlobalFlags) *cobra.Command     { return &cobra.Command{Use: "msg"} }
func newThreadCommand(*GlobalFlags) *cobra.Command  { return &cobra.Command{Use: "thread"} }
func newSearchCommand(*GlobalFlags) *cobra.Command  { return &cobra.Command{Use: "search"} }
func newCanvasCommand(*GlobalFlags) *cobra.Command  { return &cobra.Command{Use: "canvas"} }
func newListCommand(*GlobalFlags) *cobra.Command    { return &cobra.Command{Use: "list"} }
func newChannelCommand(*GlobalFlags) *cobra.Command { return &cobra.Command{Use: "channel"} }
func newUserCommand(*GlobalFlags) *cobra.Command    { return &cobra.Command{Use: "user"} }
