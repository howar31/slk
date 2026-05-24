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
		newChannelInfoCommand(g),
		newChannelMembersCommand(g),
		newChannelJoinCommand(g),
		newChannelLeaveCommand(g),
		newChannelPurposeCommand(g),
		newChannelKickCommand(g),
		newChannelRenameCommand(g),
		newChannelUnarchiveCommand(g),
		newChannelOpenCommand(g),
		newChannelMarkCommand(g),
		newChannelCloseCommand(g),
	)
	return cmd
}

func newChannelListCommand(g *GlobalFlags) *cobra.Command {
	var limit int
	var cursor, types string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List channels",
		Annotations: map[string]string{
			"slackMethod": "conversations.list",
			"userScopes":  "channels:read,groups:read,im:read,mpim:read",
			"botScopes":   "channels:read,groups:read,im:read,mpim:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// --raw is not offered here: a multi-page response has no single raw envelope.
			client, err := buildClient(cmd, g)
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
		Annotations: map[string]string{
			"slackMethod": "conversations.create",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"name": name}
			if private {
				params["is_private"] = "true"
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.create %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
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
		Annotations: map[string]string{
			"slackMethod": "conversations.archive",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.archive %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
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
		Annotations: map[string]string{
			"slackMethod": "conversations.invite",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		Long: "Invite users to a channel. Cannot invite a channel's creator or an existing member (Slack returns cant_invite_self / already_in_channel).",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "users": users}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.invite %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
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
		Annotations: map[string]string{
			"slackMethod": "conversations.setTopic",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "topic": topic}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.setTopic %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
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

// parseChannelInfo extracts the channel object from a conversations.info raw
// response and returns a single searchHit.
func parseChannelInfo(raw []byte) (searchHit, error) {
	var resp struct {
		Channel struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			NumMembers int    `json:"num_members"`
			IsArchived bool   `json:"is_archived"`
		} `json:"channel"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return searchHit{}, err
	}
	extra := fmt.Sprintf("%d members", resp.Channel.NumMembers)
	if resp.Channel.IsArchived {
		extra += " (archived)"
	}
	return searchHit{Name: resp.Channel.Name, ID: resp.Channel.ID, Extra: extra}, nil
}

// parseChannelMembers extracts the members slice and next_cursor from a
// conversations.members raw response. It returns the hits and the cursor for
// the next page (empty string when exhausted).
func parseChannelMembers(raw []byte) ([]searchHit, string, error) {
	var resp struct {
		Members          []string `json:"members"`
		ResponseMetadata struct {
			NextCursor string `json:"next_cursor"`
		} `json:"response_metadata"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, "", err
	}
	hits := make([]searchHit, len(resp.Members))
	for i, id := range resp.Members {
		hits[i] = searchHit{ID: id}
	}
	return hits, resp.ResponseMetadata.NextCursor, nil
}

func newChannelInfoCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show channel details",
		Annotations: map[string]string{
			"slackMethod": "conversations.info",
			"userScopes":  "channels:read,groups:read,im:read,mpim:read",
			"botScopes":   "channels:read,groups:read,im:read,mpim:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.info", map[string]string{"channel": channel}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hit, err := parseChannelInfo(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, []searchHit{hit})
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newChannelMembersCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "members",
		Short: "List channel members",
		// --raw is not offered here: a multi-page response has no single raw envelope.
		Annotations: map[string]string{
			"slackMethod": "conversations.members",
			"userScopes":  "channels:read,groups:read,im:read,mpim:read",
			"botScopes":   "channels:read,groups:read,im:read,mpim:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			params := map[string]string{"channel": channel, "limit": "200"}
			var hits []searchHit
			const maxPages = 10
			for page := 0; page < maxPages; page++ {
				raw, err := client.Call("conversations.members", params, nil)
				if err != nil {
					return err
				}
				pageHits, nextCursor, err := parseChannelMembers(raw)
				if err != nil {
					return err
				}
				hits = append(hits, pageHits...)
				if nextCursor == "" {
					break
				}
				params["cursor"] = nextCursor
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newChannelJoinCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "join",
		Short: "Join a channel",
		Annotations: map[string]string{
			"slackMethod": "conversations.join",
			"write":       "true",
			"userScopes":  "channels:write",
			"botScopes":   "channels:join",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.join %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.join", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "joined")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newChannelLeaveCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "leave",
		Short: "Leave a channel",
		Annotations: map[string]string{
			"slackMethod": "conversations.leave",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.leave %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.leave", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "left")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newChannelPurposeCommand(g *GlobalFlags) *cobra.Command {
	var channel, purpose string
	cmd := &cobra.Command{
		Use:   "purpose",
		Short: "Set a channel's purpose",
		Annotations: map[string]string{
			"slackMethod": "conversations.setPurpose",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "purpose": purpose}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.setPurpose %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.setPurpose", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "purpose set")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&purpose, "purpose", "", "new purpose")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("purpose")
	return cmd
}

func newChannelKickCommand(g *GlobalFlags) *cobra.Command {
	var channel, user string
	cmd := &cobra.Command{
		Use:   "kick",
		Short: "Remove a user from a channel",
		Annotations: map[string]string{
			"slackMethod": "conversations.kick",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "user": user}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.kick %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.kick", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "kicked")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&user, "user", "", "user ID to remove")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("user")
	return cmd
}

func newChannelRenameCommand(g *GlobalFlags) *cobra.Command {
	var channel, name string
	cmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a channel",
		Annotations: map[string]string{
			"slackMethod": "conversations.rename",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "name": name}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.rename %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.rename", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "renamed")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&name, "name", "", "new channel name")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("name")
	return cmd
}

func newChannelUnarchiveCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "unarchive",
		Short: "Unarchive a channel",
		Annotations: map[string]string{
			"slackMethod": "conversations.unarchive",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.unarchive %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.unarchive", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "unarchived")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newChannelOpenCommand(g *GlobalFlags) *cobra.Command {
	var users string
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open or create a direct/group message channel",
		Annotations: map[string]string{
			"slackMethod": "conversations.open",
			"write":       "true",
			"userScopes":  "im:write,mpim:write",
			"botScopes":   "im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"users": users}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.open %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.open", params, nil)
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
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "opened %s\n", resp.Channel.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&users, "users", "", "comma-separated user IDs")
	cmd.MarkFlagRequired("users")
	return cmd
}

func newChannelMarkCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts string
	cmd := &cobra.Command{
		Use:   "mark",
		Short: "Move the read cursor in a channel",
		Annotations: map[string]string{
			"slackMethod": "conversations.mark",
			"write":       "true",
			"userScopes":  "channels:write,groups:write,im:write,mpim:write",
			"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "ts": ts}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.mark %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.mark", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "marked")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "timestamp to mark as read")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	return cmd
}

func newChannelCloseCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "close",
		Short: "Close a direct/group message channel",
		Annotations: map[string]string{
			"slackMethod": "conversations.close",
			"write":       "true",
			"userScopes":  "im:write,mpim:write",
			"botScopes":   "im:write,mpim:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.close %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.close", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "closed")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.MarkFlagRequired("channel")
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
