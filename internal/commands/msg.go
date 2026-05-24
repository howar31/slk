package commands

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/howar31/slk/internal/output"
	"github.com/howar31/slk/internal/resolve"
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

// slackMessage is the subset of fields slk renders from
// conversations.history / conversations.replies. Bot-posted messages leave
// `user` empty and populate `username` and/or `bot_profile.name` instead.
type slackMessage struct {
	User       string `json:"user"`
	Username   string `json:"username"`
	BotID      string `json:"bot_id"`
	BotProfile struct {
		Name string `json:"name"`
	} `json:"bot_profile"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
}

// messageDisplay picks the best human-visible name for a message.
// Order: resolved user → username → bot_profile.name → bot_id.
func messageDisplay(r *resolve.Resolver, m slackMessage) string {
	if m.User != "" {
		return resolveUser(r, m.User)
	}
	if m.Username != "" {
		return m.Username
	}
	if m.BotProfile.Name != "" {
		return m.BotProfile.Name
	}
	return m.BotID
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
		newMsgUpdateCommand(g),
		newMsgDeleteCommand(g),
		newMsgReactCommand(g),
		newMsgUnreactCommand(g),
		newMsgScheduleCommand(g),
		newMsgUnscheduleCommand(g),
		newMsgScheduledCommand(g),
		newMsgDraftCommand(g),
		newMsgPermalinkCommand(g),
		newMsgEphemeralCommand(g),
		newMsgMeCommand(g),
		newMsgReactionsCommand(g),
		newMsgReactedCommand(g),
	)
	return cmd
}

func newMsgReadCommand(g *GlobalFlags) *cobra.Command {
	var channel, oldest, latest, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read messages from a channel or DM",
		Annotations: map[string]string{
			"slackMethod": "conversations.history",
			"userScopes":  "channels:history,groups:history,im:history,mpim:history",
			"botScopes":   "channels:history,groups:history,im:history,mpim:history",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
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
				Messages []slackMessage `json:"messages"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			r := newResolver(g, client)
			items := make([]msgItem, len(resp.Messages))
			for i, m := range resp.Messages {
				items[i] = msgItem{
					User:   messageDisplay(r, m),
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
	var channel, text, textFile, threadTS string
	var replyBroadcast bool
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message to a channel or DM",
		Annotations: map[string]string{
			"slackMethod": "chat.postMessage",
			"write":       "true",
			"userScopes":  "chat:write",
			"botScopes":   "chat:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readContent(text, textFile, "--text", "--text-file")
			if err != nil {
				return err
			}
			params := map[string]string{"channel": channel, "text": content}
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
			client, err := buildClient(cmd, g)
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
	cmd.Flags().StringVar(&textFile, "text-file", "", "path to text file (use - for stdin)")
	cmd.Flags().StringVar(&threadTS, "thread", "", "reply in this thread ts")
	cmd.Flags().BoolVar(&replyBroadcast, "reply-broadcast", false, "also broadcast a threaded reply to the channel (requires --thread)")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newMsgDeleteCommand(g *GlobalFlags) *cobra.Command {
	values := map[string]*string{}
	method := "chat.delete"
	flags := []string{"channel", "ts"}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "delete a message",
		Annotations: map[string]string{
			"slackMethod": method,
			"write":       "true",
			"userScopes":  "chat:write",
			"botScopes":   "chat:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{}
			for _, f := range flags {
				params[f] = *values[f]
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s %v\n", method, params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call(method, params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "delete ok")
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

func newMsgUpdateCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts, text, textFile string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "update a message",
		Annotations: map[string]string{
			"slackMethod": "chat.update",
			"write":       "true",
			"userScopes":  "chat:write",
			"botScopes":   "chat:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readContent(text, textFile, "--text", "--text-file")
			if err != nil {
				return err
			}
			params := map[string]string{"channel": channel, "ts": ts, "text": content}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.update %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("chat.update", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "update ok")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "message timestamp")
	cmd.Flags().StringVar(&text, "text", "", "new message text")
	cmd.Flags().StringVar(&textFile, "text-file", "", "path to text file (use - for stdin)")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	return cmd
}

func newMsgReactCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts, emoji string
	cmd := &cobra.Command{
		Use:   "react",
		Short: "Add an emoji reaction to a message",
		Annotations: map[string]string{
			"slackMethod": "reactions.add",
			"write":       "true",
			"userScopes":  "reactions:write",
			"botScopes":   "reactions:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "timestamp": ts, "name": emoji}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] reactions.add %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("reactions.add", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
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
			Type:     "rich_text_section",
			Elements: []blockText{{Type: "text", Text: text}},
		}},
	}}
	b, _ := json.Marshal(blocks)
	return string(b)
}

func newMsgScheduleCommand(g *GlobalFlags) *cobra.Command {
	var channel, text, textFile, at, thread string
	var replyBroadcast bool
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Schedule a message for a future time",
		Annotations: map[string]string{
			"slackMethod": "chat.scheduleMessage",
			"write":       "true",
			"userScopes":  "chat:write",
			"botScopes":   "chat:write",
			"botCapable":  "true",
		},
		Long: "Schedule a message. chat.deleteScheduledMessage may return ok=true for schedules within ~5 minutes of post_at yet the message still posts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readContent(text, textFile, "--text", "--text-file")
			if err != nil {
				return err
			}
			params := map[string]string{"channel": channel, "text": content, "post_at": at}
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
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("chat.scheduleMessage", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			id := parseScheduledMessageID(raw)
			if id != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "scheduled %s at %s\n", id, at)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "scheduled")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&text, "text", "", "message text")
	cmd.Flags().StringVar(&textFile, "text-file", "", "path to text file (use - for stdin)")
	cmd.Flags().StringVar(&at, "at", "", "Unix timestamp to post at")
	cmd.Flags().StringVar(&thread, "thread", "", "optional thread parent ts")
	cmd.Flags().BoolVar(&replyBroadcast, "reply-broadcast", false, "also broadcast a threaded reply to the channel (requires --thread)")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("at")
	return cmd
}

// parseScheduledMessageID extracts scheduled_message_id from a
// chat.scheduleMessage response. Needed to cancel via
// chat.deleteScheduledMessage; Slack returns the ID only at create time.
func parseScheduledMessageID(raw []byte) string {
	var resp struct {
		ID string `json:"scheduled_message_id"`
	}
	_ = json.Unmarshal(raw, &resp)
	return resp.ID
}

func newMsgUnreactCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts, emoji string
	cmd := &cobra.Command{
		Use:   "unreact",
		Short: "Remove an emoji reaction from a message",
		Annotations: map[string]string{
			"slackMethod": "reactions.remove",
			"write":       "true",
			"userScopes":  "reactions:write",
			"botScopes":   "reactions:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "timestamp": ts, "name": emoji}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] reactions.remove %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("reactions.remove", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "unreacted")
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

func newMsgUnscheduleCommand(g *GlobalFlags) *cobra.Command {
	var channel, id string
	cmd := &cobra.Command{
		Use:   "unschedule",
		Short: "Cancel a scheduled message",
		Long:  "Cancel a scheduled message. chat.deleteScheduledMessage may return ok=true for schedules within ~5 minutes of post_at yet the message still posts.",
		Annotations: map[string]string{
			"slackMethod": "chat.deleteScheduledMessage",
			"write":       "true",
			"userScopes":  "chat:write",
			"botScopes":   "chat:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "scheduled_message_id": id}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.deleteScheduledMessage %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("chat.deleteScheduledMessage", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "unscheduled")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&id, "id", "", "scheduled message ID")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("id")
	return cmd
}

// parseMsgScheduled parses a chat.scheduledMessages.list response into searchHit rows.
func parseMsgScheduled(raw []byte) ([]searchHit, error) {
	var resp struct {
		ScheduledMessages []struct {
			ID        string `json:"id"`
			ChannelID string `json:"channel_id"`
			PostAt    int64  `json:"post_at"`
			Text      string `json:"text"`
		} `json:"scheduled_messages"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, len(resp.ScheduledMessages))
	for i, m := range resp.ScheduledMessages {
		hits[i] = searchHit{
			Name:  m.Text,
			ID:    m.ID,
			Extra: fmt.Sprintf("%d", m.PostAt),
		}
	}
	return hits, nil
}

func newMsgScheduledCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "scheduled",
		Short: "List scheduled messages",
		Annotations: map[string]string{
			"slackMethod": "chat.scheduledMessages.list",
			"userScopes":  "",
			"botScopes":   "",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			params := map[string]string{}
			if channel != "" {
				params["channel"] = channel
			}
			raw, err := client.Call("chat.scheduledMessages.list", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parseMsgScheduled(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "filter by channel ID (optional)")
	return cmd
}

func newMsgPermalinkCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts string
	cmd := &cobra.Command{
		Use:   "permalink",
		Short: "Get the permalink for a message",
		Annotations: map[string]string{
			"slackMethod": "chat.getPermalink",
			"userScopes":  "",
			"botScopes":   "",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			params := map[string]string{"channel": channel, "message_ts": ts}
			raw, err := client.Call("chat.getPermalink", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Permalink string `json:"permalink"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), resp.Permalink)
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "message timestamp")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	return cmd
}

func newMsgEphemeralCommand(g *GlobalFlags) *cobra.Command {
	var channel, user, text, textFile string
	cmd := &cobra.Command{
		Use:   "ephemeral",
		Short: "Send an ephemeral message visible only to the target user",
		Annotations: map[string]string{
			"slackMethod": "chat.postEphemeral",
			"write":       "true",
			"userScopes":  "chat:write",
			"botScopes":   "chat:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readContent(text, textFile, "--text", "--text-file")
			if err != nil {
				return err
			}
			params := map[string]string{"channel": channel, "user": user, "text": content}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.postEphemeral %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("chat.postEphemeral", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "sent ephemeral")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&user, "user", "", "user ID of the recipient")
	cmd.Flags().StringVar(&text, "text", "", "message text")
	cmd.Flags().StringVar(&textFile, "text-file", "", "path to text file (use - for stdin)")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("user")
	return cmd
}

func newMsgMeCommand(g *GlobalFlags) *cobra.Command {
	var channel, text string
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Send a /me message (italicized action text)",
		Annotations: map[string]string{
			"slackMethod": "chat.meMessage",
			"write":       "true",
			"userScopes":  "chat:write",
			"botScopes":   "chat:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "text": text}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.meMessage %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("chat.meMessage", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "sent")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&text, "text", "", "action text")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("text")
	return cmd
}

// parseMsgReactions parses a reactions.get response and returns the reactions
// on the message as searchHit rows.
func parseMsgReactions(raw []byte) ([]searchHit, error) {
	var resp struct {
		Message struct {
			Reactions []struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			} `json:"reactions"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, len(resp.Message.Reactions))
	for i, r := range resp.Message.Reactions {
		hits[i] = searchHit{
			Name:  r.Name,
			Extra: fmt.Sprintf("%d", r.Count),
		}
	}
	return hits, nil
}

func newMsgReactionsCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts string
	cmd := &cobra.Command{
		Use:   "reactions",
		Short: "List reactions on a message",
		Annotations: map[string]string{
			"slackMethod": "reactions.get",
			"userScopes":  "reactions:read",
			"botScopes":   "reactions:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			params := map[string]string{"channel": channel, "timestamp": ts}
			raw, err := client.Call("reactions.get", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parseMsgReactions(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "message timestamp")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	return cmd
}

// parseMsgReacted parses a reactions.list response and returns reacted items as searchHit rows.
func parseMsgReacted(raw []byte) ([]searchHit, error) {
	var resp struct {
		Items []struct {
			Type    string `json:"type"`
			Message struct {
				TS string `json:"ts"`
			} `json:"message"`
			File struct {
				ID string `json:"id"`
			} `json:"file"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, len(resp.Items))
	for i, item := range resp.Items {
		var id string
		switch item.Type {
		case "message":
			id = item.Message.TS
		case "file":
			id = item.File.ID
		default:
			id = ""
		}
		hits[i] = searchHit{
			ID:    id,
			Extra: item.Type,
		}
	}
	return hits, nil
}

func newMsgReactedCommand(g *GlobalFlags) *cobra.Command {
	var user string
	cmd := &cobra.Command{
		Use:   "reacted",
		Short: "List items the user has reacted to",
		Annotations: map[string]string{
			"slackMethod": "reactions.list",
			"userScopes":  "reactions:read",
			"botScopes":   "reactions:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			params := map[string]string{}
			if user != "" {
				params["user"] = user
			}
			raw, err := client.Call("reactions.list", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parseMsgReacted(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "user ID (defaults to the authed user when empty)")
	return cmd
}

func newMsgDraftCommand(g *GlobalFlags) *cobra.Command {
	var channel, text, textFile, thread string
	cmd := &cobra.Command{
		Use:   "draft",
		Short: "Create a message draft via drafts.create",
		Annotations: map[string]string{
			"slackMethod": "drafts.create",
			"write":       "true",
			"userScopes":  "",
			"botScopes":   "",
			"botCapable":  "false",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readContent(text, textFile, "--text", "--text-file")
			if err != nil {
				return err
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] drafts.create channel_id=%s thread=%s text=%q\n", channel, thread, content)
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
				"blocks":           textToBlocks(content),
				"file_ids":         "[]",
				"is_from_composer": "true",
			}

			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("drafts.create", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Draft struct {
					ID     string `json:"id"`
					TeamID string `json:"team_id"`
				} `json:"draft"`
			}
			_ = json.Unmarshal(raw, &resp)
			if resp.Draft.TeamID != "" {
				fmt.Fprintf(cmd.OutOrStdout(),
					"draft created %s (manage in Slack: https://app.slack.com/client/%s/%s)\n",
					resp.Draft.ID, resp.Draft.TeamID, channel)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "draft created %s\n", resp.Draft.ID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "destination channel or user ID")
	cmd.Flags().StringVar(&text, "text", "", "draft message text")
	cmd.Flags().StringVar(&textFile, "text-file", "", "path to text file (use - for stdin)")
	cmd.Flags().StringVar(&thread, "thread", "", "optional thread_ts for a draft reply")
	cmd.MarkFlagRequired("channel")
	return cmd
}
