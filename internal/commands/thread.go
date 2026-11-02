package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newThreadCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "thread", Short: "Read and reply to threads"}
	cmd.AddCommand(newThreadReadCommand(g), newThreadReplyCommand(g))
	return cmd
}

// threadTSFromRef returns the ts of the thread a message URL belongs to.
func threadTSFromRef(ref messageRef) (string, error) {
	if ref.ThreadTS != "" {
		return ref.ThreadTS, nil
	}
	if ref.TS != "" {
		return ref.TS, nil
	}
	return "", fmt.Errorf("--url must point at a message, not a channel")
}

func newThreadReadCommand(g *GlobalFlags) *cobra.Command {
	var channel, thread, messageURL, oldest, latest, cursor, imagesDir string
	var limit int
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read replies in a thread",
		Long: `Reads a thread. Output lists each message's attached files with their IDs.
--url reads the thread a message URL belongs to, from any message in it.

--save-images downloads attached images into a directory and reports each
local path. It needs the files:read scope. Images already in the directory are
reused. An image that cannot be saved is reported on its message and the
command exits 1 after printing the output.`,
		Annotations: map[string]string{
			"slackMethod": "conversations.replies",
			"userScopes":  "channels:history,groups:history,im:history,mpim:history",
			"botScopes":   "channels:history,groups:history,im:history,mpim:history",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if imagesDir != "" && g.Raw {
				return fmt.Errorf("--save-images cannot be combined with --raw")
			}
			if messageURL != "" {
				ref, err := parseMessageURL(messageURL)
				if err != nil {
					return err
				}
				if thread, err = threadTSFromRef(ref); err != nil {
					return err
				}
				channel = ref.Channel
			}
			client, err := buildClient(cmd, g)
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
				items[i] = msgItem{User: messageDisplay(r, m), Text: m.Text, TS: m.TS, Files: msgFiles(m.Files)}
			}
			return emitMessages(cmd, g, client, items, imagesDir)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID (required unless --url is set)")
	cmd.Flags().StringVar(&thread, "thread", "", "parent message ts (required unless --url is set)")
	cmd.Flags().StringVar(&messageURL, "url", "", "Slack message URL (instead of --channel and --thread)")
	cmd.Flags().IntVar(&limit, "limit", 100, "max replies")
	cmd.Flags().StringVar(&oldest, "oldest", "", "start of time range (ts)")
	cmd.Flags().StringVar(&latest, "latest", "", "end of time range (ts)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().StringVar(&imagesDir, "save-images", "", "directory to save attached images into")
	cmd.MarkFlagsRequiredTogether("channel", "thread")
	cmd.MarkFlagsOneRequired("channel", "url")
	cmd.MarkFlagsMutuallyExclusive("channel", "url")
	cmd.MarkFlagsMutuallyExclusive("thread", "url")
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
			"userScopes":  "chat:write",
			"botScopes":   "chat:write",
			"botCapable":  "true",
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
