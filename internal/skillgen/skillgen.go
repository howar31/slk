// Package skillgen renders the agent-facing SKILL.md from the Cobra command
// tree, so the skill is a generated artifact rather than a hand-maintained file.
package skillgen

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

//go:embed skill.md.tmpl
var docTemplate string

type tmplData struct {
	Version      string
	GlobalFlags  string
	CommandIndex string
	Commands     string
}

// Generate renders the full SKILL.md for root at the given version.
func Generate(root *cobra.Command, version string) (string, error) {
	t, err := template.New("skill").Parse(docTemplate)
	if err != nil {
		return "", err
	}
	data := tmplData{
		Version:      version,
		GlobalFlags:  renderGlobalFlags(root),
		CommandIndex: renderIndex(root),
		Commands:     renderCommands(root),
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// visibleChildren returns the documentable subcommands of c, skipping hidden,
// unavailable, and Cobra's built-in help/completion commands.
func visibleChildren(c *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, ch := range c.Commands() {
		if ch.Hidden || !ch.IsAvailableCommand() || ch.Name() == "help" || ch.Name() == "completion" {
			continue
		}
		out = append(out, ch)
	}
	return out
}

func escapePipes(s string) string { return strings.ReplaceAll(s, "|", "\\|") }

func defaultCell(f *pflag.Flag) string {
	if f.DefValue == "" || f.DefValue == "false" {
		return "—"
	}
	return "`" + f.DefValue + "`"
}

func renderGlobalFlags(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString("| Flag | Default | Description |\n|------|---------|-------------|\n")
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		fmt.Fprintf(&b, "| `--%s` | %s | %s |\n", f.Name, defaultCell(f), escapePipes(f.Usage))
	})
	return b.String()
}

func renderIndex(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString("| Command | Description |\n|---------|-------------|\n")
	for _, c := range visibleChildren(root) {
		if c.HasAvailableSubCommands() {
			for _, sub := range visibleChildren(c) {
				fmt.Fprintf(&b, "| `%s` | %s |\n", sub.CommandPath(), escapePipes(sub.Short))
			}
		} else {
			fmt.Fprintf(&b, "| `%s` | %s |\n", c.CommandPath(), escapePipes(c.Short))
		}
	}
	return b.String()
}

func renderCommands(root *cobra.Command) string {
	var b strings.Builder
	for _, c := range visibleChildren(root) {
		if c.HasAvailableSubCommands() {
			fmt.Fprintf(&b, "## %s\n\n%s\n\n", c.CommandPath(), c.Short)
			for _, sub := range visibleChildren(c) {
				writeLeaf(&b, sub, "###")
			}
		} else {
			writeLeaf(&b, c, "##")
		}
	}
	return b.String()
}

func renderFlags(c *cobra.Command) string {
	var rows strings.Builder
	c.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		req := "—"
		if _, ok := f.Annotations[cobra.BashCompOneRequiredFlag]; ok {
			req = "✓"
		}
		fmt.Fprintf(&rows, "| `--%s` | %s | %s | %s |\n", f.Name, req, defaultCell(f), escapePipes(f.Usage))
	})
	if rows.Len() == 0 {
		return ""
	}
	return "| Flag | Required | Default | Description |\n|------|----------|---------|-------------|\n" + rows.String()
}

// firstParagraph returns the first blank-line-delimited paragraph of s with all
// internal whitespace collapsed to single spaces, so a command's Long help
// renders as one clean inline Tips line regardless of how it is wrapped.
func firstParagraph(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "\n\n"); idx >= 0 {
		s = s[:idx]
	}
	return strings.Join(strings.Fields(s), " ")
}

func writeLeaf(b *strings.Builder, c *cobra.Command, level string) {
	fmt.Fprintf(b, "%s %s\n\n%s\n\n", level, c.CommandPath(), c.Short)
	if m := c.Annotations["slackMethod"]; m != "" {
		fmt.Fprintf(b, "**Slack API:** `%s`\n\n", m)
	}
	fmt.Fprintf(b, "```bash\n%s\n```\n\n", c.UseLine())
	if tbl := renderFlags(c); tbl != "" {
		b.WriteString(tbl)
		b.WriteString("\n")
	}
	if c.Example != "" {
		fmt.Fprintf(b, "**Examples:**\n\n```bash\n%s\n```\n\n", strings.TrimSpace(c.Example))
	}
	if tip := firstParagraph(c.Long); tip != "" && tip != strings.TrimSpace(c.Short) {
		fmt.Fprintf(b, "**Tips:** %s\n\n", tip)
	}
	if c.Annotations["write"] == "true" {
		b.WriteString("> [!CAUTION]\n> Write command — confirm with the user before executing; preview with `--dry-run`.\n\n")
	}
}
