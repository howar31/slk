package skillgen_test

import (
	"strings"
	"testing"

	"github.com/howar31/slk/internal/skillgen"
	"github.com/spf13/cobra"
)

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
		Long:        "List channels.\n\nThis second paragraph must not appear in Tips.",
		Annotations: map[string]string{"slackMethod": "conversations.list"},
		Run:         func(*cobra.Command, []string) {},
	}
	ch.AddCommand(read)
	root.AddCommand(ch)

	// A top-level leaf command (no subcommands) — must fold into the index.
	api := &cobra.Command{Use: "api", Short: "Call any Slack Web API method directly", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(api)
	return root
}

func mustGen(t *testing.T) map[string]string {
	t.Helper()
	files, err := skillgen.GenerateAll(newTree(), "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestGenerateAll_FileSet(t *testing.T) {
	files := mustGen(t)
	for _, want := range []string{"slk/SKILL.md", "slk-shared/SKILL.md", "slk-channel/SKILL.md"} {
		if _, ok := files[want]; !ok {
			t.Errorf("missing generated file %q", want)
		}
	}
	// `api` is a top-level leaf and must NOT get its own skill dir.
	if _, ok := files["slk-api/SKILL.md"]; ok {
		t.Error("top-level leaf `api` must fold into the index, not get its own skill")
	}
}

func TestGenerateAll_InstallBlockOnlyInIndex(t *testing.T) {
	files := mustGen(t)
	if !strings.Contains(files["slk/SKILL.md"], `package: "@howar31/slk"`) {
		t.Error("index skill must carry the install block")
	}
	for _, p := range []string{"slk-shared/SKILL.md", "slk-channel/SKILL.md"} {
		if strings.Contains(files[p], `package: "@howar31/slk"`) {
			t.Errorf("%s must NOT duplicate the install block", p)
		}
	}
}

func TestGenerateAll_IndexHasGroupDirectoryAndLeaves(t *testing.T) {
	idx := mustGen(t)["slk/SKILL.md"]
	if !strings.Contains(idx, "[slk-channel](../slk-channel/SKILL.md)") {
		t.Error("index must link each group to its sub-skill")
	}
	if !strings.Contains(idx, "## slk api") {
		t.Error("index must render top-level leaf bodies (api)")
	}
}

func TestGenerateAll_GroupHasPrerequisiteAndVerbs(t *testing.T) {
	ch := mustGen(t)["slk-channel/SKILL.md"]
	for _, want := range []string{
		"name: slk-channel",
		"version: 9.9.9",
		`cliHelp: "slk channel --help"`,
		"../slk-shared/SKILL.md",
		"# slk channel",
		"## slk channel invite",
		"**Slack API:** `conversations.invite`",
		"| `--channel` | ✓ |",
		"[!CAUTION]",
		"| `slk channel invite` |",
	} {
		if !strings.Contains(ch, want) {
			t.Errorf("slk-channel skill missing %q", want)
		}
	}
	// Read verb must not get a write CAUTION; Tips must be first paragraph only.
	listSection := ch[strings.Index(ch, "## slk channel list"):]
	if strings.Contains(listSection, "[!CAUTION]") {
		t.Error("read command `channel list` should not have a CAUTION callout")
	}
	if !strings.Contains(ch, "**Tips:** List channels.") {
		t.Error("Tips should contain the first paragraph of Long")
	}
	if strings.Contains(ch, "second paragraph must not appear") {
		t.Error("Tips must not include paragraphs beyond the first")
	}
}

func TestGenerateAll_SharedHasGlobalFlagsNotGroups(t *testing.T) {
	files := mustGen(t)
	if !strings.Contains(files["slk-shared/SKILL.md"], "output format: concise\\|json") {
		t.Error("shared skill must carry the global flags table (with escaped pipe)")
	}
	if strings.Contains(files["slk-channel/SKILL.md"], "output format: concise") {
		t.Error("group skills must not duplicate the global flags table")
	}
}

func TestGenerateAll_Idempotent(t *testing.T) {
	a, err := skillgen.GenerateAll(newTree(), "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	b, err := skillgen.GenerateAll(newTree(), "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("file count differs: %d vs %d", len(a), len(b))
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || va != vb {
			t.Errorf("non-idempotent output for %q", k)
		}
	}
}
