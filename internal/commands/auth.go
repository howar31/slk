package commands

import (
	"fmt"
	"net/url"

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
	)
	return cmd
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
	cmd := &cobra.Command{
		Use:   "set-token",
		Short: "Store tokens for a profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p := cfg.Profiles[profile]
			if workspace != "" {
				p.Workspace = workspace
			}
			if userToken != "" {
				p.UserToken = userToken
			}
			if botToken != "" {
				p.BotToken = botToken
			}
			cfg.Profiles[profile] = p
			if cfg.Active == "" {
				cfg.Active = profile
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved profile %q\n", profile)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "default", "profile name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace label")
	cmd.Flags().StringVar(&userToken, "user", "", "user token (xoxp-)")
	cmd.Flags().StringVar(&botToken, "bot", "", "bot token (xoxb-)")
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
	cmd.Flags().StringVar(&scopes, "scopes", "channels:history,channels:read,chat:write,users:read", "comma-separated user scopes")
	cmd.Flags().StringVar(&port, "port", "3000", "local callback port")
	cmd.MarkFlagRequired("client-id")
	cmd.MarkFlagRequired("client-secret")
	return cmd
}
