package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newTeamCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "team", Short: "Inspect the workspace"}
	cmd.AddCommand(newTeamInfoCommand(g), newTeamProfileCommand(g))
	return cmd
}

// parseTeamInfo extracts team fields from a team.info raw response and returns
// a single searchHit with domain as the Extra field.
func parseTeamInfo(raw []byte) (searchHit, error) {
	var resp struct {
		Team struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Domain string `json:"domain"`
		} `json:"team"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return searchHit{}, err
	}
	return searchHit{
		Name:  resp.Team.Name,
		ID:    resp.Team.ID,
		Extra: resp.Team.Domain + ".slack.com",
	}, nil
}

func newTeamInfoCommand(g *GlobalFlags) *cobra.Command {
	var team string
	cmd := &cobra.Command{
		Use:         "info",
		Short:       "Show workspace info",
		Annotations: map[string]string{
			"slackMethod": "team.info",
			"userScopes":  "team:read",
			"botScopes":   "team:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			params := map[string]string{}
			if team != "" {
				params["team"] = team
			}
			raw, err := client.Call("team.info", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hit, err := parseTeamInfo(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, []searchHit{hit})
		},
	}
	cmd.Flags().StringVar(&team, "team", "", "team ID (defaults to the token's workspace)")
	return cmd
}

// parseTeamProfile extracts profile fields from a team.profile.get raw response
// and returns a slice of searchHit — one per custom profile field.
func parseTeamProfile(raw []byte) ([]searchHit, error) {
	var resp struct {
		Profile struct {
			Fields []struct {
				ID    string `json:"id"`
				Label string `json:"label"`
				Type  string `json:"type"`
			} `json:"fields"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, len(resp.Profile.Fields))
	for i, f := range resp.Profile.Fields {
		hits[i] = searchHit{Name: f.Label, ID: f.ID, Extra: f.Type}
	}
	return hits, nil
}

func newTeamProfileCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "profile",
		Short:       "List workspace profile fields",
		Annotations: map[string]string{
			"slackMethod": "team.profile.get",
			"userScopes":  "users.profile:read",
			"botScopes":   "users.profile:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("team.profile.get", map[string]string{}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parseTeamProfile(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}
