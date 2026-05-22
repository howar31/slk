package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newThreadCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "thread", Short: "Read and reply to threads"}
	cmd.AddCommand(newThreadReadCommand(g), newThreadReplyCommand(g))
	return cmd
}

func newThreadReadCommand(g *GlobalFlags) *cobra.Command {
	var channel, thread, oldest, latest, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:         "read",
		Short:       "Read replies in a thread",
		Annotations: map[string]string{"slackMethod": "conversations.replies"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			params := map[string]string{
				"channel": channel,
				"ts":      thread,
				"limit":   fmt.Sprintf("%d", limit),
			}
			if oldest != "" {
				params["oldest"] = oldest
			}
			if latest != "" {
				params["latest"] = latest
			}
			if cursor != "" {
				params["cursor"] = cursor
			}
			raw, err := client.Call("conversations.replies", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Messages []slackMessage `json:"messages"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			r := newResolver(g, client)
			items := make([]msgItem, len(resp.Messages))
			for i, m := range resp.Messages {
				items[i] = msgItem{User: messageDisplay(r, m), Text: m.Text, TS: m.TS}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&thread, "thread", "", "parent message ts")
	cmd.Flags().IntVar(&limit, "limit", 100, "max replies")
	cmd.Flags().StringVar(&oldest, "oldest", "", "start of time range (ts)")
	cmd.Flags().StringVar(&latest, "latest", "", "end of time range (ts)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("thread")
	return cmd
}

func newThreadReplyCommand(g *GlobalFlags) *cobra.Command {
	var channel, thread, text, textFile string
	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Reply within a thread",
		Annotations: map[string]string{
			"slackMethod": "chat.postMessage",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readContent(text, textFile, "--text", "--text-file")
			if err != nil {
				return err
			}
			params := map[string]string{"channel": channel, "thread_ts": thread, "text": content}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.postMessage %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("chat.postMessage", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "replied")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&thread, "thread", "", "parent message ts")
	cmd.Flags().StringVar(&text, "text", "", "reply text")
	cmd.Flags().StringVar(&textFile, "text-file", "", "path to text file (use - for stdin)")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("thread")
	return cmd
}
