package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newChannelCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "channel", Short: "List and manage channels"}
	cmd.AddCommand(
		newChannelListCommand(g),
		newChannelCreateCommand(g),
		newChannelArchiveCommand(g),
		newChannelInviteCommand(g),
		newChannelTopicCommand(g),
	)
	return cmd
}

func newChannelListCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
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

func newChannelCreateCommand(g *GlobalFlags) *cobra.Command {
	var name string
	var private bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"name": name}
			if private {
				params["is_private"] = "true"
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.create %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.create", params, nil)
			if err != nil {
				return err
			}
			var resp struct {
				Channel struct {
					ID string `json:"id"`
				} `json:"channel"`
			}
			json.Unmarshal(raw, &resp)
			fmt.Fprintf(cmd.OutOrStdout(), "created channel %s\n", resp.Channel.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "channel name")
	cmd.Flags().BoolVar(&private, "private", false, "create a private channel")
	cmd.MarkFlagRequired("name")
	return cmd
}

func newChannelArchiveCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive a channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.archive %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("conversations.archive", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "archived")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newChannelInviteCommand(g *GlobalFlags) *cobra.Command {
	var channel, users string
	cmd := &cobra.Command{
		Use:   "invite",
		Short: "Invite users to a channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "users": users}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.invite %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("conversations.invite", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "invited")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&users, "users", "", "comma-separated user IDs")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("users")
	return cmd
}

func newChannelTopicCommand(g *GlobalFlags) *cobra.Command {
	var channel, topic string
	cmd := &cobra.Command{
		Use:   "topic",
		Short: "Set a channel's topic",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "topic": topic}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.setTopic %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("conversations.setTopic", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "topic set")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&topic, "topic", "", "new topic")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("topic")
	return cmd
}
