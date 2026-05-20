package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/api"
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
	var limit int
	var cursor, types string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List channels",
		RunE: func(cmd *cobra.Command, args []string) error {
			// --raw is not offered here: a multi-page response has no single raw envelope.
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			hits, err := fetchChannelsWith(client, channelListOpts{
				Types:           types,
				Cursor:          cursor,
				Limit:           limit,
				ExcludeArchived: false,
			})
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max channels to return (0 = no client-side cap)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "initial pagination cursor")
	cmd.Flags().StringVar(&types, "types", "public_channel,private_channel", "conversations.list types param")
	return cmd
}

// channelListOpts captures the user-facing knobs of newChannelListCommand. It
// stays internal so fetchChannels (used by search.channels) keeps its smaller
// signature.
type channelListOpts struct {
	Types           string
	Cursor          string
	Limit           int
	ExcludeArchived bool
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
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Channel struct {
					ID string `json:"id"`
				} `json:"channel"`
			}
			_ = json.Unmarshal(raw, &resp)
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
			raw, err := client.Call("conversations.archive", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
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
			raw, err := client.Call("conversations.invite", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
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
			raw, err := client.Call("conversations.setTopic", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
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

// fetchChannels pages through conversations.list and returns trimmed channel
// hits using slk's default knobs. Kept for callers (search.channels) that do
// not surface pagination flags.
func fetchChannels(client *api.Client, excludeArchived bool) ([]searchHit, error) {
	return fetchChannelsWith(client, channelListOpts{ExcludeArchived: excludeArchived})
}

// fetchChannelsWith pages through conversations.list honoring caller-supplied
// types / cursor / total-limit knobs. limit==0 means "no client-side cap".
// Pagination stops as soon as `limit` results have accumulated so a tight
// `--limit` does not pay for unused pages.
func fetchChannelsWith(client *api.Client, opts channelListOpts) ([]searchHit, error) {
	types := opts.Types
	if types == "" {
		types = "public_channel,private_channel"
	}
	params := map[string]string{
		"limit": "200",
		"types": types,
	}
	if opts.ExcludeArchived {
		params["exclude_archived"] = "true"
	}
	cursor := opts.Cursor
	var hits []searchHit
	const maxPages = 10
	for page := 0; page < maxPages; page++ {
		if cursor != "" {
			params["cursor"] = cursor
		} else {
			delete(params, "cursor")
		}
		raw, err := client.Call("conversations.list", params, nil)
		if err != nil {
			return hits, err
		}
		var resp struct {
			Channels []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				NumMembers int    `json:"num_members"`
			} `json:"channels"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return hits, err
		}
		for _, c := range resp.Channels {
			hits = append(hits, searchHit{
				Name:  c.Name,
				ID:    c.ID,
				Extra: fmt.Sprintf("%d members", c.NumMembers),
			})
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
