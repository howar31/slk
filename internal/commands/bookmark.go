package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newBookmarkCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "bookmark", Short: "Manage channel bookmarks"}
	cmd.AddCommand(
		newBookmarkAddCommand(g),
		newBookmarkEditCommand(g),
		newBookmarkRemoveCommand(g),
		newBookmarkListCommand(g),
	)
	return cmd
}

func newBookmarkAddCommand(g *GlobalFlags) *cobra.Command {
	var channel, title, link, typ string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a bookmark to a channel",
		Annotations: map[string]string{
			"slackMethod": "bookmarks.add",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{
				"channel_id": channel,
				"title":      title,
				"link":       link,
				"type":       typ,
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] bookmarks.add %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("bookmarks.add", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "bookmark added")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&title, "title", "", "bookmark title")
	cmd.Flags().StringVar(&link, "link", "", "bookmark URL")
	cmd.Flags().StringVar(&typ, "type", "link", "bookmark type")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("title")
	cmd.MarkFlagRequired("link")
	return cmd
}

func newBookmarkEditCommand(g *GlobalFlags) *cobra.Command {
	var channel, id, title, link string
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing channel bookmark",
		Annotations: map[string]string{
			"slackMethod": "bookmarks.edit",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{
				"channel_id":  channel,
				"bookmark_id": id,
			}
			// Only include optional params that were explicitly set.
			if cmd.Flags().Changed("title") {
				params["title"] = title
			}
			if cmd.Flags().Changed("link") {
				params["link"] = link
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] bookmarks.edit %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("bookmarks.edit", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "bookmark updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&id, "id", "", "bookmark ID")
	cmd.Flags().StringVar(&title, "title", "", "new bookmark title")
	cmd.Flags().StringVar(&link, "link", "", "new bookmark URL")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newBookmarkRemoveCommand(g *GlobalFlags) *cobra.Command {
	var channel, id string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a bookmark from a channel",
		Annotations: map[string]string{
			"slackMethod": "bookmarks.remove",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{
				"channel_id":  channel,
				"bookmark_id": id,
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] bookmarks.remove %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("bookmarks.remove", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "bookmark removed")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&id, "id", "", "bookmark ID")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newBookmarkListCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List bookmarks in a channel",
		Annotations: map[string]string{"slackMethod": "bookmarks.list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("bookmarks.list", map[string]string{"channel_id": channel}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parseBookmarks(raw)
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

// parseBookmarks extracts bookmark entries from a bookmarks.list raw response.
// Each bookmark becomes searchHit{Name: title, ID: id, Extra: link}.
func parseBookmarks(raw []byte) ([]searchHit, error) {
	var resp struct {
		Bookmarks []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Link  string `json:"link"`
		} `json:"bookmarks"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, len(resp.Bookmarks))
	for i, b := range resp.Bookmarks {
		hits[i] = searchHit{Name: b.Title, ID: b.ID, Extra: b.Link}
	}
	return hits, nil
}
