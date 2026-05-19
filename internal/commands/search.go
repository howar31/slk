package commands

import (
	"encoding/json"
	"fmt"

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
	)
	return cmd
}

func newSearchMessagesCommand(g *GlobalFlags) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:   "messages",
		Short: "Search messages (requires a user token)",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("search.messages", map[string]string{"query": query}, nil)
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
	cmd.MarkFlagRequired("query")
	return cmd
}

func newSearchChannelsCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channels",
		Short: "List channels",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			pages, err := client.CallAll("conversations.list",
				map[string]string{"limit": "200", "types": "public_channel,private_channel"}, 10)
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
						Name: c.Name, ID: c.ID,
						Extra: fmt.Sprintf("%d members", c.NumMembers),
					})
				}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}

func newSearchUsersCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List users",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			pages, err := client.CallAll("users.list", map[string]string{"limit": "200"}, 10)
			if err != nil {
				return err
			}
			var hits []searchHit
			for _, raw := range pages {
				var resp struct {
					Members []struct {
						ID       string `json:"id"`
						Name     string `json:"name"`
						RealName string `json:"real_name"`
					} `json:"members"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return err
				}
				for _, m := range resp.Members {
					hits = append(hits, searchHit{Name: m.Name, ID: m.ID, Extra: m.RealName})
				}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}
