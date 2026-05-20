package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

// userProfile holds trimmed fields from users.profile.get.
type userProfile struct {
	DisplayName string `json:"display_name"`
	RealName    string `json:"real_name"`
	Title       string `json:"title"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	StatusText  string `json:"status_text,omitempty"`
	StatusEmoji string `json:"status_emoji,omitempty"`
	TZ          string `json:"tz,omitempty"`
}

func (p userProfile) Concise() string {
	s := p.DisplayName
	if p.RealName != "" && p.RealName != p.DisplayName {
		s = fmt.Sprintf("%s (%s)", p.DisplayName, p.RealName)
	}
	if p.Title != "" {
		s += " — " + p.Title
	}
	if p.StatusText != "" {
		s += "  ·  " + p.StatusEmoji + " " + p.StatusText
	}
	return s
}

func newUserCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "List and inspect users"}
	cmd.AddCommand(newUserListCommand(g), newUserInfoCommand(g), newUserProfileCommand(g))
	return cmd
}

func newUserListCommand(g *GlobalFlags) *cobra.Command {
	var limit int
	var cursor string
	var includeBots, includeDeactivated bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspace users",
		Long: `List workspace users.

By default the output excludes bot users and deactivated accounts, which are
usually noise for agent workflows. Pass --include-bots / --include-deactivated
to bring them back. --raw is not offered here: a multi-page response has no
single raw envelope.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			hits, err := fetchUsersWith(client, userListOpts{
				Cursor:             cursor,
				Limit:              limit,
				IncludeBots:        includeBots,
				IncludeDeactivated: includeDeactivated,
			})
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max users to return (0 = no client-side cap)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "initial pagination cursor")
	cmd.Flags().BoolVar(&includeBots, "include-bots", false, "include bot users (default false)")
	cmd.Flags().BoolVar(&includeDeactivated, "include-deactivated", false, "include deactivated users (default false)")
	return cmd
}

// userListOpts captures the user-facing knobs of newUserListCommand.
type userListOpts struct {
	Cursor             string
	Limit              int
	IncludeBots        bool
	IncludeDeactivated bool
}

// fetchUsers pages through users.list and returns trimmed user hits. Kept
// unfiltered for `search users`, which performs its own client-side narrowing
// via --query and should not silently drop bots / deactivated accounts.
func fetchUsers(client *api.Client) ([]searchHit, error) {
	return fetchUsersWith(client, userListOpts{IncludeBots: true, IncludeDeactivated: true})
}

// fetchUsersWith pages through users.list honoring caller-supplied
// pagination + filter knobs. Pagination stops once Limit results have
// accumulated so a tight --limit does not pay for unused pages.
func fetchUsersWith(client *api.Client, opts userListOpts) ([]searchHit, error) {
	params := map[string]string{"limit": "200"}
	cursor := opts.Cursor
	var hits []searchHit
	const maxPages = 10
	for page := 0; page < maxPages; page++ {
		if cursor != "" {
			params["cursor"] = cursor
		} else {
			delete(params, "cursor")
		}
		raw, err := client.Call("users.list", params, nil)
		if err != nil {
			return hits, err
		}
		var resp struct {
			Members []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				RealName string `json:"real_name"`
				IsBot    bool   `json:"is_bot"`
				Deleted  bool   `json:"deleted"`
			} `json:"members"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return hits, err
		}
		for _, m := range resp.Members {
			// Slackbot is flagged is_bot=false by the API but is functionally a
			// system bot — treat it as one for filtering purposes.
			isBot := m.IsBot || m.ID == "USLACKBOT"
			if isBot && !opts.IncludeBots {
				continue
			}
			if m.Deleted && !opts.IncludeDeactivated {
				continue
			}
			hits = append(hits, searchHit{Name: m.Name, ID: m.ID, Extra: m.RealName})
			if opts.Limit > 0 && len(hits) >= opts.Limit {
				return hits, nil
			}
		}
		cursor = resp.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return hits, nil
}

func newUserInfoCommand(g *GlobalFlags) *cobra.Command {
	var userID string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show one user's profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("users.info", map[string]string{"user": userID}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				User struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					RealName string `json:"real_name"`
				} `json:"user"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			hit := searchHit{Name: resp.User.Name, ID: resp.User.ID, Extra: resp.User.RealName}
			return output.Emit(cmd.OutOrStdout(), g.Format, []searchHit{hit})
		},
	}
	cmd.Flags().StringVar(&userID, "user", "", "user ID")
	cmd.MarkFlagRequired("user")
	return cmd
}

func newUserProfileCommand(g *GlobalFlags) *cobra.Command {
	var userID string
	var includeLocale bool
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Show a user's profile fields",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			params := map[string]string{}
			if userID != "" {
				params["user"] = userID
			}
			if includeLocale {
				params["include_locale"] = "true"
			}
			raw, err := client.Call("users.profile.get", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Profile struct {
					DisplayName string `json:"display_name"`
					RealName    string `json:"real_name"`
					Title       string `json:"title"`
					Email       string `json:"email"`
					Phone       string `json:"phone"`
					StatusText  string `json:"status_text"`
					StatusEmoji string `json:"status_emoji"`
					TZ          string `json:"tz"`
				} `json:"profile"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			p := userProfile{
				DisplayName: resp.Profile.DisplayName,
				RealName:    resp.Profile.RealName,
				Title:       resp.Profile.Title,
				Email:       resp.Profile.Email,
				Phone:       resp.Profile.Phone,
				StatusText:  resp.Profile.StatusText,
				StatusEmoji: resp.Profile.StatusEmoji,
				TZ:          resp.Profile.TZ,
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, []userProfile{p})
		},
	}
	cmd.Flags().StringVar(&userID, "user", "", "user ID (defaults to current user when empty)")
	cmd.Flags().BoolVar(&includeLocale, "include-locale", false, "include locale in response")
	return cmd
}
