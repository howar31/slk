package commands

import (
	"encoding/json"
	"fmt"

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
