package commands

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newDndCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "dnd", Short: "Do-Not-Disturb status and snooze"}
	cmd.AddCommand(
		newDndInfoCommand(g),
		newDndTeamCommand(g),
		newDndSnoozeCommand(g),
		newDndEndSnoozeCommand(g),
		newDndEndCommand(g),
	)
	return cmd
}

// dndInfo holds trimmed fields from a dnd.info response.
type dndInfo struct {
	DndEnabled    bool `json:"dnd_enabled"`
	SnoozeEnabled bool `json:"snooze_enabled"`
	NextDndStart  int  `json:"next_dnd_start_ts"`
	NextDndEnd    int  `json:"next_dnd_end_ts"`
}

func (d dndInfo) Concise() string {
	return fmt.Sprintf("dnd_enabled=%v snooze_enabled=%v", d.DndEnabled, d.SnoozeEnabled)
}

// parseDndInfo extracts dnd status fields from a raw dnd.info response.
func parseDndInfo(raw []byte) (dndInfo, error) {
	var resp struct {
		DndEnabled    bool `json:"dnd_enabled"`
		SnoozeEnabled bool `json:"snooze_enabled"`
		NextDndStart  int  `json:"next_dnd_start_ts"`
		NextDndEnd    int  `json:"next_dnd_end_ts"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return dndInfo{}, err
	}
	return dndInfo{
		DndEnabled:    resp.DndEnabled,
		SnoozeEnabled: resp.SnoozeEnabled,
		NextDndStart:  resp.NextDndStart,
		NextDndEnd:    resp.NextDndEnd,
	}, nil
}

// parseDndTeam parses a dnd.teamInfo response into a slice of searchHit sorted
// by user ID. Each hit carries dnd_enabled in Extra.
func parseDndTeam(raw []byte) ([]searchHit, error) {
	var resp struct {
		Users map[string]struct {
			DndEnabled    bool `json:"dnd_enabled"`
			SnoozeEnabled bool `json:"snooze_enabled"`
		} `json:"users"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, 0, len(resp.Users))
	for uid, info := range resp.Users {
		hits = append(hits, searchHit{
			ID:    uid,
			Extra: fmt.Sprintf("dnd_enabled=%v", info.DndEnabled),
		})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].ID < hits[j].ID })
	return hits, nil
}

func newDndInfoCommand(g *GlobalFlags) *cobra.Command {
	var userID string
	cmd := &cobra.Command{
		Use:         "info",
		Short:       "Show DND status for a user (or the caller)",
		Annotations: map[string]string{"slackMethod": "dnd.info"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			params := map[string]string{}
			if userID != "" {
				params["user"] = userID
			}
			raw, err := client.Call("dnd.info", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			info, err := parseDndInfo(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, []dndInfo{info})
		},
	}
	cmd.Flags().StringVar(&userID, "user", "", "user ID (omit for the calling user)")
	return cmd
}

func newDndTeamCommand(g *GlobalFlags) *cobra.Command {
	var users string
	cmd := &cobra.Command{
		Use:         "team",
		Short:       "Show DND status for a comma-separated list of users",
		Annotations: map[string]string{"slackMethod": "dnd.teamInfo"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("dnd.teamInfo", map[string]string{"users": users}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parseDndTeam(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&users, "users", "", "comma-separated user IDs")
	cmd.MarkFlagRequired("users")
	return cmd
}

func newDndSnoozeCommand(g *GlobalFlags) *cobra.Command {
	var minutes int
	cmd := &cobra.Command{
		Use:   "snooze",
		Short: "Start a DND snooze for the given number of minutes",
		Annotations: map[string]string{
			"slackMethod": "dnd.setSnooze",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"num_minutes": fmt.Sprintf("%d", minutes)}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] dnd.setSnooze %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("dnd.setSnooze", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "snoozing")
			return nil
		},
	}
	cmd.Flags().IntVar(&minutes, "minutes", 0, "number of minutes to snooze")
	cmd.MarkFlagRequired("minutes")
	return cmd
}

func newDndEndSnoozeCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "end-snooze",
		Short: "End the active DND snooze",
		Annotations: map[string]string{
			"slackMethod": "dnd.endSnooze",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] dnd.endSnooze %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("dnd.endSnooze", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "snooze ended")
			return nil
		},
	}
	return cmd
}

func newDndEndCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "end",
		Short: "End the active DND period",
		Annotations: map[string]string{
			"slackMethod": "dnd.endDnd",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] dnd.endDnd %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("dnd.endDnd", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "dnd ended")
			return nil
		},
	}
	return cmd
}
