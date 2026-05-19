package commands

import (
	"crypto/rand"
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
		newMsgDraftCommand(g),
	)
	return cmd
}

func newMsgReadCommand(g *GlobalFlags) *cobra.Command {
	var channel, oldest, latest, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read messages from a channel or DM",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			params := map[string]string{
				"channel": channel,
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
			raw, err := client.Call("conversations.history", params, nil)
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
	cmd.Flags().StringVar(&oldest, "oldest", "", "start of time range (ts)")
	cmd.Flags().StringVar(&latest, "latest", "", "end of time range (ts)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newMsgSendCommand(g *GlobalFlags) *cobra.Command {
	var channel, text, threadTS string
	var replyBroadcast bool
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message to a channel or DM",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "text": text}
			if threadTS != "" {
				params["thread_ts"] = threadTS
			}
			if replyBroadcast && threadTS != "" {
				params["reply_broadcast"] = "true"
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
	cmd.Flags().BoolVar(&replyBroadcast, "reply-broadcast", false, "also broadcast a threaded reply to the channel (requires --thread)")
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

// newUUID generates a random UUID v4 string using crypto/rand.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Block types for textToBlocks.
type blockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type blockSection struct {
	Type     string      `json:"type"`
	Elements []blockText `json:"elements"`
}
type richTextBlock struct {
	Type     string         `json:"type"`
	Elements []blockSection `json:"elements"`
}

// textToBlocks wraps plain text in a Slack rich_text block JSON array.
func textToBlocks(text string) string {
	blocks := []richTextBlock{{
		Type: "rich_text",
		Elements: []blockSection{{
			Type: "rich_text_section",
			Elements: []blockText{{Type: "text", Text: text}},
		}},
	}}
	b, _ := json.Marshal(blocks)
	return string(b)
}

func newMsgScheduleCommand(g *GlobalFlags) *cobra.Command {
	var channel, text, at, thread string
	var replyBroadcast bool
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Schedule a message for a future time",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "text": text, "post_at": at}
			if thread != "" {
				params["thread_ts"] = thread
			}
			if replyBroadcast && thread != "" {
				params["reply_broadcast"] = "true"
			}
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
	cmd.Flags().StringVar(&thread, "thread", "", "optional thread parent ts")
	cmd.Flags().BoolVar(&replyBroadcast, "reply-broadcast", false, "also broadcast a threaded reply to the channel (requires --thread)")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("text")
	cmd.MarkFlagRequired("at")
	return cmd
}

func newMsgDraftCommand(g *GlobalFlags) *cobra.Command {
	var channel, text, thread string
	cmd := &cobra.Command{
		Use:   "draft",
		Short: "Create a message draft via drafts.create",
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] drafts.create channel_id=%s thread=%s text=%q\n", channel, thread, text)
				return nil
			}

			dest := map[string]string{"channel_id": channel}
			if thread != "" {
				dest["thread_ts"] = thread
			}
			destinationsBytes, _ := json.Marshal([]map[string]string{dest})

			params := map[string]string{
				"channel_id":       channel,
				"client_msg_id":    newUUID(),
				"destinations":     string(destinationsBytes),
				"blocks":           textToBlocks(text),
				"file_ids":         "[]",
				"is_from_composer": "true",
			}

			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("drafts.create", params, nil)
			if err != nil {
				return err
			}
			var resp struct {
				Draft struct {
					ID string `json:"id"`
				} `json:"draft"`
			}
			_ = json.Unmarshal(raw, &resp)
			fmt.Fprintf(cmd.OutOrStdout(), "draft created %s\n", resp.Draft.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "destination channel or user ID")
	cmd.Flags().StringVar(&text, "text", "", "draft message text")
	cmd.Flags().StringVar(&thread, "thread", "", "optional thread_ts for a draft reply")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("text")
	return cmd
}
