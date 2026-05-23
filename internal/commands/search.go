package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

// searchHit is one trimmed search/list result.
type searchHit struct {
	Name  string `json:"name"`
	ID    string `json:"id"`
	Extra string `json:"extra,omitempty"`
}

func (h searchHit) Concise() string {
	if h.Extra != "" {
		return fmt.Sprintf("%s (%s) — %s", h.Name, h.ID, h.Extra)
	}
	return fmt.Sprintf("%s (%s)", h.Name, h.ID)
}

func newSearchCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "search", Short: "Search messages, channels, users"}
	cmd.AddCommand(
		newSearchMessagesCommand(g),
		newSearchChannelsCommand(g),
		newSearchUsersCommand(g),
		newSearchFilesCommand(g),
		newSearchAllCommand(g),
	)
	return cmd
}

func newSearchMessagesCommand(g *GlobalFlags) *cobra.Command {
	var query string
	var public bool
	cmd := &cobra.Command{
		Use:         "messages",
		Short:       "Search messages (requires a user token)",
		Annotations: map[string]string{"slackMethod": "search.messages"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			finalQuery := query
			if public {
				finalQuery = strings.TrimSpace(query + " in:public")
			}
			raw, err := client.Call("search.messages", map[string]string{"query": finalQuery}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Messages struct {
					Matches []struct {
						Username string `json:"username"`
						Text     string `json:"text"`
						TS       string `json:"ts"`
					} `json:"matches"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			items := make([]msgItem, len(resp.Messages.Matches))
			for i, m := range resp.Messages.Matches {
				items[i] = msgItem{User: m.Username, Text: m.Text, TS: m.TS}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	cmd.Flags().BoolVar(&public, "public", false, "restrict the search to public channels (appends in:public to the query)")
	cmd.MarkFlagRequired("query")
	return cmd
}

func newSearchChannelsCommand(g *GlobalFlags) *cobra.Command {
	var query string
	var includeArchived bool
	var channelTypes string
	cmd := &cobra.Command{
		Use:         "channels",
		Short:       "List/search channels (client-side filter)",
		Annotations: map[string]string{"slackMethod": "conversations.list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// --raw is not offered here: a multi-page response has no single raw envelope.
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			params := map[string]string{
				"limit": "200",
				"types": channelTypes,
			}
			if !includeArchived {
				params["exclude_archived"] = "true"
			}
			pages, err := client.CallAll("conversations.list", params, 10)
			if err != nil {
				return err
			}
			var hits []searchHit
			for _, raw := range pages {
				var resp struct {
					Channels []struct {
						ID         string `json:"id"`
						Name       string `json:"name"`
						NumMembers int    `json:"num_members"`
					} `json:"channels"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return err
				}
				for _, c := range resp.Channels {
					hits = append(hits, searchHit{
						Name:  c.Name,
						ID:    c.ID,
						Extra: fmt.Sprintf("%d members", c.NumMembers),
					})
				}
			}
			if query != "" {
				q := strings.ToLower(query)
				filtered := hits[:0]
				for _, h := range hits {
					if strings.Contains(strings.ToLower(h.Name), q) {
						filtered = append(filtered, h)
					}
				}
				hits = filtered
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "filter channels whose name contains this substring (case-insensitive)")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "include archived channels")
	cmd.Flags().StringVar(&channelTypes, "channel-types", "public_channel,private_channel", "comma-separated channel types: public_channel,private_channel")
	return cmd
}

func newSearchUsersCommand(g *GlobalFlags) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:         "users",
		Short:       "List/search workspace users (client-side filter)",
		Annotations: map[string]string{"slackMethod": "users.list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// --raw is not offered here: a multi-page response has no single raw envelope.
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			hits, err := fetchUsers(client)
			if err != nil {
				return err
			}
			if query != "" {
				q := strings.ToLower(query)
				filtered := hits[:0]
				for _, h := range hits {
					if strings.Contains(strings.ToLower(h.Name), q) ||
						strings.Contains(strings.ToLower(h.Extra), q) {
						filtered = append(filtered, h)
					}
				}
				hits = filtered
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "filter users whose name/real_name contains this substring (case-insensitive)")
	return cmd
}

func newSearchFilesCommand(g *GlobalFlags) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:         "files",
		Short:       "Search files (requires a user token)",
		Annotations: map[string]string{"slackMethod": "search.files"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("search.files", map[string]string{"query": query}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			items, err := parseSearchFiles(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	cmd.MarkFlagRequired("query")
	return cmd
}

// parseSearchFiles extracts file hits from a search.files response.
// Each hit maps to searchHit{Name:name, ID:id, Extra:filetype}.
func parseSearchFiles(raw []byte) ([]searchHit, error) {
	var resp struct {
		Files struct {
			Matches []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Filetype string `json:"filetype"`
			} `json:"matches"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	items := make([]searchHit, len(resp.Files.Matches))
	for i, f := range resp.Files.Matches {
		items[i] = searchHit{Name: f.Name, ID: f.ID, Extra: f.Filetype}
	}
	return items, nil
}

func newSearchAllCommand(g *GlobalFlags) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:         "all",
		Short:       "Search messages and files combined (requires a user token)",
		Annotations: map[string]string{"slackMethod": "search.all"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("search.all", map[string]string{"query": query}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			items, err := parseSearchAll(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	cmd.MarkFlagRequired("query")
	return cmd
}

// parseSearchAll combines message and file hits from a search.all response.
// Messages → searchHit{Name:username, ID:ts, Extra:"message"}.
// Files    → searchHit{Name:name, ID:id, Extra:filetype}.
func parseSearchAll(raw []byte) ([]searchHit, error) {
	var resp struct {
		Messages struct {
			Matches []struct {
				Username string `json:"username"`
				TS       string `json:"ts"`
			} `json:"matches"`
		} `json:"messages"`
		Files struct {
			Matches []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Filetype string `json:"filetype"`
			} `json:"matches"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	var items []searchHit
	for _, m := range resp.Messages.Matches {
		items = append(items, searchHit{Name: m.Username, ID: m.TS, Extra: "message"})
	}
	for _, f := range resp.Files.Matches {
		items = append(items, searchHit{Name: f.Name, ID: f.ID, Extra: f.Filetype})
	}
	return items, nil
}
