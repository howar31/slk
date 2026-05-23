package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/howar31/slk/internal/api"
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

// OAuth flow seams — overridable in tests so login can run without a real browser
// callback or a live Slack token exchange.
var (
	waitForCode  = auth.WaitForCode
	exchangeCode = auth.ExchangeCode
)

// liveIdentity calls auth.test for token and returns the formatted identity line
// for `auth status`. Seam: overridable in tests so status runs without a live
// token or network. On failure it returns the error unchanged — an *api.APIError
// when Slack rejected the call, any other error for transport/offline failures —
// so the caller can classify it.
var liveIdentity = func(token string) (string, error) {
	c := api.New(token)
	c.HTTP.Timeout = 4 * time.Second
	raw, err := c.Call("auth.test", nil, nil)
	if err != nil {
		return "", err
	}
	return formatAuthIdentity(raw)
}

// identitySuffix renders the live-check fragment for one profile in `auth status`:
// the resolved identity on success, or a classified note. It never reaches the
// network for a token Load could not decrypt. A Slack rejection (*api.APIError)
// reads as an invalid token; any other error reads as offline.
func identitySuffix(p auth.Profile) string {
	token := p.UserToken
	if token == "" {
		token = p.BotToken
	}
	if token == "" {
		return "(no token)"
	}
	if auth.IsEncrypted(token) {
		return "(local: token could not be decrypted)"
	}
	line, err := liveIdentity(token)
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			return fmt.Sprintf("(invalid token: %s)", apiErr.SlackError)
		}
		return "(offline: could not reach Slack)"
	}
	return line
}

// promptCtx bundles the I/O and interactivity state the auth commands resolve their
// fields against, so set-token and login share one resolution path.
type promptCtx struct {
	in          io.Reader
	errw        io.Writer
	fd          int
	interactive bool
}

// newPromptCtx derives the prompt context from a command. interactive is true only
// when not forced off and stdin is a terminal; fd comes from the input reader when it
// is an *os.File, otherwise os.Stdin.
func newPromptCtx(cmd *cobra.Command, nonInteractive bool) promptCtx {
	in := cmd.InOrStdin()
	// Derive fd from the input reader itself. A non-*os.File reader gets fd -1 so it
	// never borrows os.Stdin's terminal status (isTerminal(-1) is false), which keeps a
	// redirected/in-memory input from being mistaken for an interactive terminal.
	fd := -1
	if f, ok := in.(*os.File); ok {
		fd = int(f.Fd())
	}
	return promptCtx{
		in:          in,
		errw:        cmd.ErrOrStderr(),
		fd:          fd,
		interactive: !nonInteractive && isTerminal(fd),
	}
}

// line resolves a non-secret field (flag → prompt-if-TTY → empty).
func (pc promptCtx) line(flagVal string, changed bool, prompt string) (string, error) {
	return resolveLine(pc.in, pc.errw, flagVal, changed, pc.interactive, prompt)
}

// secret resolves a hidden field (flag/"-" stdin → hidden prompt-if-TTY → empty).
func (pc promptCtx) secret(flagVal string, changed, required bool, prompt string) (string, error) {
	return resolveSecret(pc.in, pc.errw, pc.fd, flagVal, changed, pc.interactive, required, prompt)
}

func newAuthSetTokenCommand() *cobra.Command {
	var profile, userToken, botToken string
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "set-token",
		Short: "Store tokens for a profile",
		Long: "Store tokens for a profile. Pass values via flags for scripts/agents, or run " +
			"with missing fields in a terminal to be prompted (token entry is hidden). Use " +
			"--user - / --bot - to read a token from stdin.",
		RunE: func(cmd *cobra.Command, args []string) error {
			pc := newPromptCtx(cmd, nonInteractive)
			flags := cmd.Flags()

			name, err := pc.line(profile, flags.Changed("profile"), "Profile name [default]: ")
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

			// A new profile must end up with a token; an existing one may keep its current.
			ut, err := pc.secret(userToken, flags.Changed("user"), !existed, "Paste user token (xoxp-, hidden): ")
			if err != nil {
				return err
			}
			if ut != "" {
				p.UserToken = ut
			}

			bt, err := pc.secret(botToken, flags.Changed("bot"), false, "Paste bot token (xoxb-, optional, hidden): ")
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
	cmd.Flags().StringVar(&userToken, "user", "", "user token (xoxp-, or - to read stdin)")
	cmd.Flags().StringVar(&botToken, "bot", "", "bot token (xoxb-, or - to read stdin)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "never prompt; require values via flags")
	return cmd
}

func newAuthStatusCommand() *cobra.Command {
	var all, offline bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show configured profiles",
		Long: "Show configured profiles. By default the active profile is verified live " +
			"against Slack (auth.test); pass --all to verify every profile, or --offline to " +
			"list local info only without any network call.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no profiles configured")
				return nil
			}

			// Encryption is a config-wide property (one key backend, one health
			// line for the whole file), so it leads — separated from the per-profile
			// table below.
			fmt.Fprintln(cmd.OutOrStdout(), auth.EncryptionStatus(cfg))
			fmt.Fprintln(cmd.OutOrStdout())

			// Sort by name for a stable order (BurntSushi also writes profiles
			// alphabetically, so this matches config.toml). Pad names to align the
			// columns.
			names := make([]string, 0, len(cfg.Profiles))
			width := 0
			for name := range cfg.Profiles {
				names = append(names, name)
				if len(name) > width {
					width = len(name)
				}
			}
			sort.Strings(names)

			for _, name := range names {
				p := cfg.Profiles[name]
				marker := " "
				if name == cfg.Active {
					marker = "*"
				}
				line := fmt.Sprintf("%s %-*s (user=%v bot=%v)",
					marker, width, name, p.UserToken != "", p.BotToken != "")
				if !offline && (all || name == cfg.Active) {
					line += " — " + identitySuffix(p)
				}
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			if !offline && !all && len(names) > 1 {
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), "Run with --all to verify every profile, not just the active one.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "verify every profile live, not just the active one")
	cmd.Flags().BoolVar(&offline, "offline", false, "skip the live Slack check; list local info only")
	return cmd
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
	var profile, clientID, clientSecret, scopes, port string
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Run the OAuth flow with your own Slack app credentials",
		Long: "Run the OAuth flow with your own Slack app credentials. Pass values via flags " +
			"for scripts/agents, or run with missing fields in a terminal to be prompted (the " +
			"client secret is entered hidden).",
		RunE: func(cmd *cobra.Command, args []string) error {
			pc := newPromptCtx(cmd, nonInteractive)
			flags := cmd.Flags()

			name, err := pc.line(profile, flags.Changed("profile"), "Profile name [default]: ")
			if err != nil {
				return err
			}
			if name == "" {
				name = "default"
			}

			id, err := pc.line(clientID, flags.Changed("client-id"), "Client ID: ")
			if err != nil {
				return err
			}
			secret, err := pc.secret(clientSecret, flags.Changed("client-secret"), false, "Client secret (hidden): ")
			if err != nil {
				return err
			}
			if id == "" || secret == "" {
				return fmt.Errorf("client id and secret are required; pass --client-id and --client-secret, or run in a terminal")
			}

			redirectURI := "http://localhost:" + port + "/callback"
			q := url.Values{
				"client_id":    {id},
				"user_scope":   {scopes},
				"redirect_uri": {redirectURI},
			}
			authURL := "https://slack.com/oauth/v2/authorize?" + q.Encode()
			fmt.Fprintf(cmd.OutOrStdout(), "Open this URL to authorize:\n%s\n", authURL)

			code, err := waitForCode(":"+port, "/callback")
			if err != nil {
				return err
			}
			pair, err := exchangeCode("https://slack.com/api/oauth.v2.access",
				id, secret, code, redirectURI)
			if err != nil {
				return err
			}

			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p := cfg.Profiles[name]
			if pair.UserToken != "" {
				p.UserToken = pair.UserToken
			}
			if pair.BotToken != "" {
				p.BotToken = pair.BotToken
			}
			cfg.Profiles[name] = p
			if cfg.Active == "" {
				cfg.Active = name
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "authorized profile %q\n", name)
			if !pc.interactive && !flags.Changed("profile") {
				fmt.Fprintln(cmd.ErrOrStderr(), "  (saved to the default profile; pass --profile to choose another)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "default", "profile name")
	cmd.Flags().StringVar(&clientID, "client-id", "", "your Slack app client ID")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "your Slack app client secret")
	cmd.Flags().StringVar(&scopes, "scopes", "channels:history,channels:read,channels:write,groups:history,groups:read,groups:write,im:history,im:read,im:write,mpim:history,mpim:read,mpim:write,chat:write,reactions:write,reactions:read,search:read,users:read,users:write,users.profile:read,users.profile:write,files:read,files:write,canvases:read,canvases:write,lists:read,lists:write,pins:read,pins:write,bookmarks:read,bookmarks:write,team:read,emoji:read,dnd:read,dnd:write,usergroups:read,usergroups:write", "comma-separated user scopes")
	cmd.Flags().StringVar(&port, "port", "3000", "local callback port")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "never prompt; require values via flags")
	return cmd
}
