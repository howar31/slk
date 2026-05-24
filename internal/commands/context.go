package commands

import "github.com/spf13/cobra"

// GlobalFlags holds flags shared by every command.
type GlobalFlags struct {
	Format    string // concise|json|jsonl|table
	Identity  string // user|bot
	Profile   string
	Raw       bool
	DryRun    bool
	NoResolve bool
}

// bindGlobalFlags registers persistent flags on cmd, writing into g.
func bindGlobalFlags(cmd *cobra.Command, g *GlobalFlags) {
	pf := cmd.PersistentFlags()
	pf.StringVar(&g.Format, "format", "concise", "output format: concise|json|jsonl|table")
	pf.StringVar(&g.Identity, "as", "", "identity user|bot: on a command, assert the active token's scope; on 'auth login', which token to mint (default user)")
	pf.StringVar(&g.Profile, "profile", "", "config profile to use")
	pf.BoolVar(&g.Raw, "raw", false, "return raw Slack API response")
	pf.BoolVar(&g.DryRun, "dry-run", false, "validate without calling the API")
	pf.BoolVar(&g.NoResolve, "no-resolve", false, "do not resolve IDs to names")
}
