package commands

import (
	"os"

	"github.com/howar31/slk/internal/skillgen"
	"github.com/spf13/cobra"
)

// newGenerateSkillCommand builds the hidden `generate-skill` command, which
// renders skills/slk/SKILL.md from the live command tree. version is stamped
// into the skill's metadata.version.
func newGenerateSkillCommand(version string) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:    "generate-skill",
		Short:  "Generate skills/slk/SKILL.md from the command tree",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			md, err := skillgen.Generate(cmd.Root(), version)
			if err != nil {
				return err
			}
			return os.WriteFile(output, []byte(md), 0o644)
		},
	}
	cmd.Flags().StringVar(&output, "output", "skills/slk/SKILL.md", "output path for the generated SKILL.md")
	return cmd
}
