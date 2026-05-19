package commands

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

// msgItem is one trimmed message.
type msgItem struct {
	User   string `json:"user"`
	Text   string `json:"text"`
	TS     string `json:"ts"`
	Thread string `json:"thread,omitempty"`
}

func (m msgItem) Concise() string {
	return fmt.Sprintf("%s: %s [%s]", m.User, m.Text, shortTS(m.TS))
}

// shortTS renders a Slack ts (e.g. "1779191572.123") as "MM-DD HH:MM".
func shortTS(ts string) string {
	var sec int64
	fmt.Sscanf(ts, "%d", &sec)
	if sec == 0 {
		return ts
	}
	return time.Unix(sec, 0).Format("01-02 15:04")
}

func newMsgCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "msg", Short: "Read and send messages"}
	cmd.AddCommand(
		newMsgReadCommand(g),
		newMsgSendCommand(g),
		newMsgWriteCommand(g, "update", "chat.update", []string{"channel", "ts", "text"}),
		newMsgWriteCommand(g, "delete", "chat.delete", []string{"channel", "ts"}),
		newMsgReactCommand(g),
		newMsgScheduleCommand(g),
	)
	return cmd
}

func newMsgReadCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	var limit int
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read messages from a channel or DM",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.history", map[string]string{
				"channel": channel,
				"limit":   fmt.Sprintf("%d", limit),
			}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Messages []struct {
					User     string `json:"user"`
					Text     string `json:"text"`
					TS       string `json:"ts"`
					ThreadTS string `json:"thread_ts"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			r := newResolver(g, client)
			items := make([]msgItem, len(resp.Messages))
			for i, m := range resp.Messages {
				items[i] = msgItem{
					User:   resolveUser(r, m.User),
					Text:   m.Text,
					TS:     m.TS,
					Thread: m.ThreadTS,
				}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID or user ID for a DM")
	cmd.Flags().IntVar(&limit, "limit", 50, "max messages")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newMsgSendCommand(g *GlobalFlags) *cobra.Command {
	var channel, text, threadTS string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message to a channel or DM",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "text": text}
			if threadTS != "" {
				params["thread_ts"] = threadTS
			}
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
			var resp struct {
				TS string `json:"ts"`
			}
			_ = json.Unmarshal(raw, &resp)
			fmt.Fprintf(cmd.OutOrStdout(), "sent %s\n", resp.TS)
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID or user ID")
	cmd.Flags().StringVar(&text, "text", "", "message text")
	cmd.Flags().StringVar(&threadTS, "thread", "", "reply in this thread ts")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("text")
	return cmd
}

// newMsgWriteCommand builds a simple write command mapping required flags to
// Slack form params of the same name.
func newMsgWriteCommand(g *GlobalFlags, use, method string, flags []string) *cobra.Command {
	values := map[string]*string{}
	cmd := &cobra.Command{
		Use:   use,
		Short: use + " a message",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{}
			for _, f := range flags {
				params[f] = *values[f]
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s %v\n", method, params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call(method, params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), use+" ok")
			return nil
		},
	}
	for _, f := range flags {
		v := new(string)
		values[f] = v
		cmd.Flags().StringVar(v, f, "", f+" value")
		cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newMsgReactCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts, emoji string
	cmd := &cobra.Command{
		Use:   "react",
		Short: "Add an emoji reaction to a message",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "timestamp": ts, "name": emoji}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] reactions.add %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("reactions.add", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "reacted")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "message timestamp")
	cmd.Flags().StringVar(&emoji, "emoji", "", "emoji name without colons")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	cmd.MarkFlagRequired("emoji")
	return cmd
}

func newMsgScheduleCommand(g *GlobalFlags) *cobra.Command {
	var channel, text, at string
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Schedule a message for a future time",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "text": text, "post_at": at}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.scheduleMessage %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("chat.scheduleMessage", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "scheduled")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&text, "text", "", "message text")
	cmd.Flags().StringVar(&at, "at", "", "Unix timestamp to post at")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("text")
	cmd.MarkFlagRequired("at")
	return cmd
}
