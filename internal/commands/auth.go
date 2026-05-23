package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/howar31/slk/internal/auth"
	"github.com/spf13/cobra"
)

func newAuthCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage Slack credentials"}
	cmd.AddCommand(
		newAuthSetTokenCommand(),
		newAuthStatusCommand(),
		newAuthLoginCommand(),
		newAuthSwitchCommand(),
		newAuthLogoutCommand(),
		newAuthTestCommand(g),
		newAuthRevokeCommand(g),
	)
	return cmd
}

func newAuthTestCommand(g *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "test",
		Short:       "Verify the active token and show its live identity",
		Annotations: map[string]string{"slackMethod": "auth.test"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("auth.test", nil, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			line, err := formatAuthIdentity(raw)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
			return nil
		},
	}
}

// formatAuthIdentity renders an auth.test response as a single
// "team (team_id) — user (user_id) @ url" line.
func formatAuthIdentity(raw []byte) (string, error) {
	var resp struct {
		URL    string `json:"url"`
		Team   string `json:"team"`
		User   string `json:"user"`
		TeamID string `json:"team_id"`
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s (%s) — %s (%s) @ %s",
		resp.Team, resp.TeamID, resp.User, resp.UserID, resp.URL), nil
}

func newAuthRevokeCommand(g *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke",
		Short: "Revoke the active token at Slack (server-side)",
		Annotations: map[string]string{
			"slackMethod": "auth.revoke",
			"write":       "true",
		},
		Long: "Revoke the active token at Slack. This invalidates the token server-side; " +
			"it does not remove the local profile (use `auth logout` for that).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.DryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "[dry-run] auth.revoke")
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("auth.revoke", nil, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Revoked bool `json:"revoked"`
			}
			_ = json.Unmarshal(raw, &resp)
			fmt.Fprintf(cmd.OutOrStdout(), "revoked=%v\n", resp.Revoked)
			return nil
		},
	}
}

func loadConfig() (string, *auth.Config, error) {
	path, err := auth.ConfigPath()
	if err != nil {
		return "", nil, err
	}
	cfg, err := auth.Load(path)
	return path, cfg, err
}

func newAuthSetTokenCommand() *cobra.Command {
	var profile, workspace, userToken, botToken string
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "set-token",
		Short: "Store tokens for a profile",
		Long: "Store tokens for a profile. Pass values via flags for scripts/agents, or run " +
			"with missing fields in a terminal to be prompted (token entry is hidden). Use " +
			"--user - / --bot - to read a token from stdin.",
		RunE: func(cmd *cobra.Command, args []string) error {
			in := cmd.InOrStdin()
			fd := int(os.Stdin.Fd())
			if f, ok := in.(*os.File); ok {
				fd = int(f.Fd())
			}
			interactive := !nonInteractive && isTerminal(fd)
			errw := cmd.ErrOrStderr()
			flags := cmd.Flags()

			name, err := resolveLine(in, errw, profile, flags.Changed("profile"), interactive, "Profile name [default]: ")
			if err != nil {
				return err
			}
			if name == "" {
				name = "default"
			}

			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p, existed := cfg.Profiles[name]

			ws, err := resolveLine(in, errw, workspace, flags.Changed("workspace"), interactive, "Workspace label (optional): ")
			if err != nil {
				return err
			}
			if ws != "" {
				p.Workspace = ws
			}

			// A new profile must end up with a token; an existing one may keep its current.
			ut, err := resolveSecret(in, errw, fd, userToken, flags.Changed("user"), interactive, !existed, "Paste user token (xoxp-, hidden): ")
			if err != nil {
				return err
			}
			if ut != "" {
				p.UserToken = ut
			}

			bt, err := resolveSecret(in, errw, fd, botToken, flags.Changed("bot"), interactive, false, "Paste bot token (xoxb-, optional, hidden): ")
			if err != nil {
				return err
			}
			if bt != "" {
				p.BotToken = bt
			}

			if p.UserToken == "" && p.BotToken == "" {
				return fmt.Errorf("no token; pass --user <token> or --bot <token> (use - to read stdin), or run in a terminal")
			}

			cfg.Profiles[name] = p
			if cfg.Active == "" {
				cfg.Active = name
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved profile %q\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "default", "profile name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace label")
	cmd.Flags().StringVar(&userToken, "user", "", "user token (xoxp-, or - to read stdin)")
	cmd.Flags().StringVar(&botToken, "bot", "", "bot token (xoxb-, or - to read stdin)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "never prompt; require values via flags")
	return cmd
}

func newAuthStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show configured profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no profiles configured")
				return nil
			}
			for name, p := range cfg.Profiles {
				marker := " "
				if name == cfg.Active {
					marker = "*"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s (workspace=%s user=%v bot=%v)\n",
					marker, name, p.Workspace, p.UserToken != "", p.BotToken != "")
			}
			fmt.Fprintln(cmd.OutOrStdout(), auth.EncryptionStatus(cfg))
			return nil
		},
	}
}

func newAuthSwitchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <profile>",
		Short: "Set the active profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Profiles[args[0]]; !ok {
				return fmt.Errorf("profile %q not found", args[0])
			}
			cfg.Active = args[0]
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "active profile: %s\n", args[0])
			return nil
		},
	}
}

func newAuthLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <profile>",
		Short: "Remove a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Profiles[args[0]]; !ok {
				return fmt.Errorf("profile %q not found", args[0])
			}
			delete(cfg.Profiles, args[0])
			if cfg.Active == args[0] {
				cfg.Active = ""
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed profile %q\n", args[0])
			return nil
		},
	}
}

func newAuthLoginCommand() *cobra.Command {
	var profile, workspace, clientID, clientSecret, scopes, port string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Run the OAuth flow with your own Slack app credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			redirectURI := "http://localhost:" + port + "/callback"
			q := url.Values{
				"client_id":    {clientID},
				"user_scope":   {scopes},
				"redirect_uri": {redirectURI},
			}
			authURL := "https://slack.com/oauth/v2/authorize?" + q.Encode()
			fmt.Fprintf(cmd.OutOrStdout(), "Open this URL to authorize:\n%s\n", authURL)

			code, err := auth.WaitForCode(":"+port, "/callback")
			if err != nil {
				return err
			}
			pair, err := auth.ExchangeCode("https://slack.com/api/oauth.v2.access",
				clientID, clientSecret, code, redirectURI)
			if err != nil {
				return err
			}
			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p := cfg.Profiles[profile]
			if workspace != "" {
				p.Workspace = workspace
			}
			p.ClientID = clientID
			p.ClientSecret = clientSecret
			if pair.UserToken != "" {
				p.UserToken = pair.UserToken
			}
			if pair.BotToken != "" {
				p.BotToken = pair.BotToken
			}
			cfg.Profiles[profile] = p
			if cfg.Active == "" {
				cfg.Active = profile
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "authorized profile %q\n", profile)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "default", "profile name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace label")
	cmd.Flags().StringVar(&clientID, "client-id", "", "your Slack app client ID")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "your Slack app client secret")
	cmd.Flags().StringVar(&scopes, "scopes", "channels:history,channels:read,channels:write,groups:history,groups:read,groups:write,im:history,im:read,im:write,mpim:history,mpim:read,mpim:write,chat:write,reactions:write,reactions:read,search:read,users:read,users:write,users.profile:read,users.profile:write,files:read,files:write,canvases:read,canvases:write,lists:read,lists:write,pins:read,pins:write,bookmarks:read,bookmarks:write,team:read,emoji:read,dnd:read,dnd:write,usergroups:read,usergroups:write", "comma-separated user scopes")
	cmd.Flags().StringVar(&port, "port", "3000", "local callback port")
	cmd.MarkFlagRequired("client-id")
	cmd.MarkFlagRequired("client-secret")
	return cmd
}
