package commands

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newEmojiCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "emoji", Short: "List custom emoji"}
	cmd.AddCommand(newEmojiListCommand(g))
	return cmd
}

// parseEmojiList extracts the emoji map from an emoji.list raw response and
// returns a slice of searchHit sorted by name. Alias entries are kept; their
// Extra field naturally shows the "alias:..." value.
func parseEmojiList(raw []byte) ([]searchHit, error) {
	var resp struct {
		Emoji map[string]string `json:"emoji"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, 0, len(resp.Emoji))
	for name, url := range resp.Emoji {
		hits = append(hits, searchHit{Name: name, Extra: url})
	}
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Name < hits[j].Name
	})
	return hits, nil
}

func newEmojiListCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List custom emoji",
		Annotations: map[string]string{"slackMethod": "emoji.list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("emoji.list", map[string]string{}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parseEmojiList(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}
