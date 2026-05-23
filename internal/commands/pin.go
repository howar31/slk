package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newPinCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "pin", Short: "Pin and unpin items"}
	cmd.AddCommand(
		newPinAddCommand(g),
		newPinRemoveCommand(g),
		newPinListCommand(g),
	)
	return cmd
}

func newPinAddCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Pin a message to a channel",
		Annotations: map[string]string{
			"slackMethod": "pins.add",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "timestamp": ts}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] pins.add %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("pins.add", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "pinned")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "message timestamp")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	return cmd
}

func newPinRemoveCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Unpin a message from a channel",
		Annotations: map[string]string{
			"slackMethod": "pins.remove",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "timestamp": ts}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] pins.remove %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("pins.remove", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "unpinned")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "message timestamp")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	return cmd
}

func newPinListCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List pinned items in a channel",
		Annotations: map[string]string{"slackMethod": "pins.list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("pins.list", map[string]string{"channel": channel}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parsePinItems(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.MarkFlagRequired("channel")
	return cmd
}

// parsePinItems extracts pinned items from a pins.list raw response.
// Messages yield searchHit{ID: message.ts, Extra: "message"};
// files yield searchHit{Name: file.name, ID: file.id, Extra: "file"}.
func parsePinItems(raw []byte) ([]searchHit, error) {
	var resp struct {
		Items []struct {
			Type    string `json:"type"`
			Message struct {
				Ts string `json:"ts"`
			} `json:"message"`
			File struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"file"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, 0, len(resp.Items))
	for _, item := range resp.Items {
		switch item.Type {
		case "message":
			hits = append(hits, searchHit{ID: item.Message.Ts, Extra: "message"})
		case "file":
			hits = append(hits, searchHit{Name: item.File.Name, ID: item.File.ID, Extra: "file"})
		}
	}
	return hits, nil
}
