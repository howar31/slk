package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newUserCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "List and inspect users"}
	cmd.AddCommand(newUserListCommand(g), newUserInfoCommand(g))
	return cmd
}

func newUserListCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspace users",
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
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}

// fetchUsers pages through users.list and returns trimmed user hits.
// NOTE: users.list returns ALL members including bots and deactivated
// accounts; slk does not filter them, so callers see the full membership.
func fetchUsers(client *api.Client) ([]searchHit, error) {
	pages, err := client.CallAll("users.list", map[string]string{"limit": "200"}, 10)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		for _, m := range resp.Members {
			hits = append(hits, searchHit{Name: m.Name, ID: m.ID, Extra: m.RealName})
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
