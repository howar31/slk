// Package skillgen renders the agent-facing skill tree from the Cobra command
// tree, so the skills are generated artifacts rather than hand-maintained files.
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

//go:embed templates/index.md.tmpl
var indexTemplate string

//go:embed templates/shared.md.tmpl
var sharedTemplate string

//go:embed templates/group.md.tmpl
var groupTemplate string

type indexData struct {
	Version    string
	Groups     string
	LeafBodies string
}

type sharedData struct {
	Version     string
	GlobalFlags string
}

type groupData struct {
	Version   string
	Name      string
	GroupPath string
	Short     string
	Commands  string
}

// GenerateAll renders every skill file for root at version. The returned map is
// keyed by path relative to the skills output directory (e.g. "slk/SKILL.md",
// "slk-shared/SKILL.md", "slk-channel/SKILL.md").
func GenerateAll(root *cobra.Command, version string) (map[string]string, error) {
	out := make(map[string]string)

	idx, err := render(indexTemplate, indexData{
		Version:    version,
		Groups:     renderGroupDirectory(root),
		LeafBodies: renderTopLevelLeaves(root),
	})
	if err != nil {
		return nil, err
	}
	out["slk/SKILL.md"] = idx

	sh, err := render(sharedTemplate, sharedData{
		Version:     version,
		GlobalFlags: renderGlobalFlags(root),
	})
	if err != nil {
		return nil, err
	}
	out["slk-shared/SKILL.md"] = sh

	for _, c := range visibleChildren(root) {
		if !c.HasAvailableSubCommands() {
			continue // top-level leaves fold into the index
		}
		var b strings.Builder
		b.WriteString(renderGroupCommandIndex(c))
		b.WriteString("\n")
		for _, sub := range visibleChildren(c) {
			writeLeaf(&b, sub, "##")
		}
		g, err := render(groupTemplate, groupData{
			Version:   version,
			Name:      "slk-" + c.Name(),
			GroupPath: c.CommandPath(),
			Short:     c.Short,
			Commands:  b.String(),
		})
		if err != nil {
			return nil, err
		}
		out["slk-"+c.Name()+"/SKILL.md"] = g
	}
	return out, nil
}

func render(tmpl string, data any) (string, error) {
	t, err := template.New("skill").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// renderGroupDirectory builds the index's table of command groups, each linking
// to its own sub-skill. Top-level leaf commands are excluded (they render via
// renderTopLevelLeaves).
func renderGroupDirectory(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString("| Group | Description | Skill |\n|-------|-------------|-------|\n")
	for _, c := range visibleChildren(root) {
		if !c.HasAvailableSubCommands() {
			continue
		}
		fmt.Fprintf(&b, "| `%s` | %s | [slk-%s](../slk-%s/SKILL.md) |\n",
			c.CommandPath(), escapePipes(c.Short), c.Name(), c.Name())
	}
	return b.String()
}

// renderGroupCommandIndex builds a quick-scan table of a group's verbs for the
// top of its skill, before the per-verb detail sections.
func renderGroupCommandIndex(c *cobra.Command) string {
	var b strings.Builder
	b.WriteString("| Command | Description |\n|---------|-------------|\n")
	for _, sub := range visibleChildren(c) {
		fmt.Fprintf(&b, "| `%s` | %s |\n", sub.CommandPath(), escapePipes(sub.Short))
	}
	return b.String()
}

// renderTopLevelLeaves renders the bodies of root commands that have no
// subcommands (e.g. `slk api`, `slk version`) for inclusion in the index.
func renderTopLevelLeaves(root *cobra.Command) string {
	var b strings.Builder
	for _, c := range visibleChildren(root) {
		if c.HasAvailableSubCommands() {
			continue
		}
		writeLeaf(&b, c, "##")
	}
	return b.String()
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
