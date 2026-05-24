package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/auth"
	"github.com/spf13/cobra"
)

func newAuthCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage Slack credentials"}
	cmd.AddCommand(
		newAuthSetTokenCommand(),
		newAuthStatusCommand(g),
		newAuthLoginCommand(g),
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
		Annotations: map[string]string{
			"slackMethod": "auth.test",
			"userScopes":  "",
			"botScopes":   "",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
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
			"userScopes":  "",
			"botScopes":   "",
			"botCapable":  "true",
		},
		Long: "Revoke the active token at Slack. This invalidates the token server-side; " +
			"it does not remove the local profile (use `auth logout` for that).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.DryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "[dry-run] auth.revoke")
				return nil
			}
			client, err := buildClient(cmd, g)
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

// authIdentity is the parsed auth.test identity for one profile.
type authIdentity struct {
	Team   string `json:"team"`
	TeamID string `json:"team_id"`
	User   string `json:"user"`
	UserID string `json:"user_id"`
	URL    string `json:"url"`
}

// profileStatus is the resolved status of one profile for `auth status`.
type profileStatus struct {
	Name     string        `json:"name"`
	Active   bool          `json:"active"`
	Scope    string        `json:"scope"` // user|bot|unknown|encrypted|none
	Checked  bool          `json:"checked"`
	Identity *authIdentity `json:"identity,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// profileScope derives the displayed scope of a profile's token.
func profileScope(p auth.Profile) string {
	if p.Token == "" {
		return "none"
	}
	if auth.IsEncrypted(p.Token) {
		return "encrypted"
	}
	if s := auth.TokenScope(p.Token); s != "" {
		return s
	}
	return "unknown"
}

// parseAuthIdentity parses an auth.test response into an authIdentity.
func parseAuthIdentity(raw []byte) (*authIdentity, error) {
	var id authIdentity
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, err
	}
	return &id, nil
}

// liveIdentity calls auth.test for token and returns the parsed identity. Seam:
// overridable in tests so status runs without a live token or network.
var liveIdentity = func(token string) (*authIdentity, error) {
	c := api.New(token)
	c.HTTP.Timeout = 4 * time.Second
	raw, err := c.Call("auth.test", nil, nil)
	if err != nil {
		return nil, err
	}
	return parseAuthIdentity(raw)
}

// resolveProfileStatus builds the status for one profile. When check is false no
// network call is made (Checked stays false). A Slack rejection reads as an
// invalid token; any other error reads as offline.
func resolveProfileStatus(name string, p auth.Profile, active, check bool) profileStatus {
	st := profileStatus{Name: name, Active: active, Scope: profileScope(p)}
	if !check || st.Scope == "none" || st.Scope == "encrypted" {
		return st
	}
	st.Checked = true
	id, err := liveIdentity(p.Token)
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			st.Error = fmt.Sprintf("invalid token: %s", apiErr.SlackError)
		} else {
			st.Error = "offline"
		}
		return st
	}
	st.Identity = id
	return st
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

// B3: newAuthSetTokenCommand — single --token flag with prefix validation.
func newAuthSetTokenCommand() *cobra.Command {
	var profile, token string
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "set-token",
		Short: "Store a token for a profile",
		Long: "Store a single token (user xoxp- or bot xoxb-) for a profile. Pass --token for " +
			"scripts/agents (use --token - to read from stdin), or run in a terminal to be " +
			"prompted (token entry is hidden). A profile holds exactly one token; its scope is " +
			"derived from the prefix.",
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
			p := cfg.Profiles[name]

			tok, err := pc.secret(token, flags.Changed("token"), true, "Paste token (xoxp- or xoxb-, hidden): ")
			if err != nil {
				return err
			}
			if tok == "" {
				return fmt.Errorf("no token; pass --token <token> (use - to read stdin), or run in a terminal")
			}
			if auth.TokenScope(tok) == "" {
				return fmt.Errorf("unsupported token prefix; slk accepts user (xoxp-) or bot (xoxb-) tokens only")
			}
			p.Token = tok

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
	cmd.Flags().StringVar(&token, "token", "", "token (xoxp- or xoxb-, or - to read stdin)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "never prompt; require values via flags")
	return cmd
}

// B4: newAuthStatusCommand — takes *GlobalFlags; derived [scope] label; --format json.
func newAuthStatusCommand(g *GlobalFlags) *cobra.Command {
	var all, offline bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show configured profiles",
		Long: "Show configured profiles with a derived [user]/[bot] scope label. By default the " +
			"active profile is verified live (auth.test); --all verifies every profile, --offline " +
			"skips the network. --format json emits a structured object.",
		Annotations: map[string]string{
			"slackMethod": "auth.test",
			"userScopes":  "",
			"botScopes":   "",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(cfg.Profiles) == 0 {
				if g.Format == "json" {
					fmt.Fprintln(out, `{"profiles":[]}`)
				} else {
					fmt.Fprintln(out, "no profiles configured")
				}
				return nil
			}

			names := make([]string, 0, len(cfg.Profiles))
			width := 0
			for name := range cfg.Profiles {
				names = append(names, name)
				if len(name) > width {
					width = len(name)
				}
			}
			sort.Strings(names)

			statuses := make(map[string]profileStatus, len(names))
			var mu sync.Mutex
			var wg sync.WaitGroup
			for _, name := range names {
				check := !offline && (all || name == cfg.Active)
				wg.Add(1)
				go func(name string, p auth.Profile) {
					defer wg.Done()
					s := resolveProfileStatus(name, p, name == cfg.Active, check)
					mu.Lock()
					statuses[name] = s
					mu.Unlock()
				}(name, cfg.Profiles[name])
			}
			wg.Wait()

			if g.Format == "json" {
				ordered := make([]profileStatus, 0, len(names))
				for _, name := range names {
					ordered = append(ordered, statuses[name])
				}
				payload := struct {
					Encryption string          `json:"encryption"`
					Active     string          `json:"active"`
					Profiles   []profileStatus `json:"profiles"`
				}{auth.EncryptionStatus(cfg), cfg.Active, ordered}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}

			fmt.Fprintln(out, auth.EncryptionStatus(cfg))
			fmt.Fprintln(out)
			for _, name := range names {
				st := statuses[name]
				marker := " "
				if st.Active {
					marker = "*"
				}
				line := fmt.Sprintf("%s %-*s [%s]", marker, width, name, st.Scope)
				switch {
				case st.Identity != nil:
					line += fmt.Sprintf(" — %s (%s) — %s (%s) @ %s",
						st.Identity.Team, st.Identity.TeamID, st.Identity.User, st.Identity.UserID, st.Identity.URL)
				case st.Error != "":
					line += " — (" + st.Error + ")"
				}
				fmt.Fprintln(out, line)
			}
			if !offline && !all && len(names) > 1 {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "Run with --all to verify every profile, not just the active one.")
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

// B5: newAuthLoginCommand — takes *GlobalFlags; --as selects mint; stores single token.
func newAuthLoginCommand(g *GlobalFlags) *cobra.Command {
	var profile, clientID, clientSecret, scopes, port string
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Run the OAuth flow with your own Slack app credentials",
		Long: "Run the OAuth flow with your own Slack app credentials. Pass values via flags " +
			"for scripts/agents, or run with missing fields in a terminal to be prompted (the " +
			"client secret is entered hidden). Use --as bot to mint a bot token instead of a " +
			"user token.",
		Annotations: map[string]string{
			"slackMethod": "oauth.v2.access",
			"userScopes":  "",
			"botScopes":   "",
			"botCapable":  "true",
		},
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

			mint := g.Identity
			if mint == "" {
				mint = "user"
			}
			if mint != "user" && mint != "bot" {
				return fmt.Errorf("--as must be user or bot")
			}

			scopeList := scopes
			if !flags.Changed("scopes") {
				scopeList = strings.Join(scopeUnion(cmd.Root(), mint), ",")
			}

			redirectURI := "http://localhost:" + port + "/callback"
			q := url.Values{
				"client_id":    {id},
				"redirect_uri": {redirectURI},
			}
			if mint == "bot" {
				q.Set("scope", scopeList)
			} else {
				q.Set("user_scope", scopeList)
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

			tok := pair.UserToken
			if mint == "bot" {
				tok = pair.BotToken
			}
			if tok == "" {
				return fmt.Errorf("oauth returned no %s token (for --as bot, your app must have a bot user)", mint)
			}

			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p := cfg.Profiles[name]
			p.Token = tok
			cfg.Profiles[name] = p
			if cfg.Active == "" {
				cfg.Active = name
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "authorized profile %q (%s)\n", name, mint)
			if !pc.interactive && !flags.Changed("profile") {
				fmt.Fprintln(cmd.ErrOrStderr(), "  (saved to the default profile; pass --profile to choose another)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "default", "profile name")
	cmd.Flags().StringVar(&clientID, "client-id", "", "your Slack app client ID")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "your Slack app client secret")
	cmd.Flags().StringVar(&scopes, "scopes", "", "comma-separated scopes to request (default: generated from supported commands for the chosen identity)")
	cmd.Flags().StringVar(&port, "port", "3000", "local callback port")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "never prompt; require values via flags")
	return cmd
}
