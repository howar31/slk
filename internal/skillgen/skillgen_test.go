package skillgen_test

import (
	"strings"
	"testing"

	"github.com/howar31/slk/internal/skillgen"
	"github.com/spf13/cobra"
)

func TestGenerate_TipsFirstParagraphAndGroupPath(t *testing.T) {
	root := &cobra.Command{Use: "slk"}
	grp := &cobra.Command{Use: "msg", Short: "Messages"}
	cmd := &cobra.Command{
		Use:         "send",
		Short:       "Send a message",
		Long:        "Send a message.\n\nThis second paragraph must not appear in Tips.",
		Annotations: map[string]string{"slackMethod": "chat.postMessage", "write": "true"},
		Run:         func(*cobra.Command, []string) {},
	}
	grp.AddCommand(cmd)
	root.AddCommand(grp)

	out, err := skillgen.Generate(root, "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "**Tips:** Send a message.") {
		t.Error("Tips should contain the first paragraph of Long")
	}
	if strings.Contains(out, "second paragraph must not appear") {
		t.Error("Tips must not include paragraphs beyond the first")
	}
	if !strings.Contains(out, "## slk msg") {
		t.Error("group heading should use the full command path (## slk msg)")
	}
}

func newTree() *cobra.Command {
	root := &cobra.Command{Use: "slk", Short: "Agent-facing Slack CLI"}
	root.PersistentFlags().String("format", "concise", "output format: concise|json")
	root.PersistentFlags().Bool("raw", false, "return raw Slack API response")

	ch := &cobra.Command{Use: "channel", Short: "List and manage channels"}
	invite := &cobra.Command{
		Use:         "invite",
		Short:       "Invite users to a channel",
		Annotations: map[string]string{"slackMethod": "conversations.invite", "write": "true"},
		Run:         func(*cobra.Command, []string) {},
	}
	invite.Flags().String("channel", "", "channel ID")
	_ = invite.MarkFlagRequired("channel")
	invite.Flags().String("users", "", "comma-separated user IDs")
	_ = invite.MarkFlagRequired("users")
	ch.AddCommand(invite)

	read := &cobra.Command{
		Use:         "list",
		Short:       "List channels",
		Annotations: map[string]string{"slackMethod": "conversations.list"},
		Run:         func(*cobra.Command, []string) {},
	}
	ch.AddCommand(read)
	root.AddCommand(ch)
	return root
}

func TestGenerate_FrontmatterAndVersion(t *testing.T) {
	out, err := skillgen.Generate(newTree(), "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: slk",
		"version: 9.9.9",
		`cliHelp: "slk --help"`,
		`package: "@howar31/slk"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestGenerate_CommandSections(t *testing.T) {
	out, err := skillgen.Generate(newTree(), "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## slk channel",
		"### slk channel invite",
		"**Slack API:** `conversations.invite`",
		"| `--channel` | ✓ |",
		"[!CAUTION]",
		"| `slk channel invite` |",
		"output format: concise\\|json",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
	listSection := out[strings.Index(out, "### slk channel list"):]
	if strings.Contains(listSection, "[!CAUTION]") {
		t.Error("read command channel list should not have a CAUTION callout")
	}
}
