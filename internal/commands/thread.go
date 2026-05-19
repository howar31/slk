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
	var channel, ts, oldest, latest, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read replies in a thread",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			params := map[string]string{
				"channel": channel,
				"ts":      ts,
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
				Messages []struct {
					User string `json:"user"`
					Text string `json:"text"`
					TS   string `json:"ts"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			r := newResolver(g, client)
			items := make([]msgItem, len(resp.Messages))
			for i, m := range resp.Messages {
				items[i] = msgItem{User: resolveUser(r, m.User), Text: m.Text, TS: m.TS}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "parent message ts")
	cmd.Flags().IntVar(&limit, "limit", 100, "max replies")
	cmd.Flags().StringVar(&oldest, "oldest", "", "start of time range (ts)")
	cmd.Flags().StringVar(&latest, "latest", "", "end of time range (ts)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	return cmd
}

func newThreadReplyCommand(g *GlobalFlags) *cobra.Command {
	var channel, thread, text string
	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Reply within a thread",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "thread_ts": thread, "text": text}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.postMessage %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("chat.postMessage", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "replied")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&thread, "thread", "", "parent message ts")
	cmd.Flags().StringVar(&text, "text", "", "reply text")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("thread")
	cmd.MarkFlagRequired("text")
	return cmd
}
