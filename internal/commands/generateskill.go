package commands

import (
	"os"
	"path/filepath"

	"github.com/howar31/slk/internal/skillgen"
	"github.com/spf13/cobra"
)

// newGenerateSkillsCommand builds the hidden `generate-skills` command, which
// renders the skill tree (index + shared + one per command group) from the live
// command tree into outputDir. version is stamped into each skill's
// metadata.version. Previously generated `slk`/`slk-*` dirs are removed first so
// a deleted command group leaves no stale skill behind.
func newGenerateSkillsCommand(version string) *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:    "generate-skills",
		Short:  "Generate the skills/ tree from the command tree",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := skillgen.GenerateAll(cmd.Root(), version)
			if err != nil {
				return err
			}
			if err := cleanGeneratedSkills(outputDir); err != nil {
				return err
			}
			for rel, content := range files {
				dst := filepath.Join(outputDir, rel)
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&outputDir, "output-dir", "skills", "output directory for the generated skill tree")
	return cmd
}

// cleanGeneratedSkills removes the previously generated skill dirs (`slk` and
// `slk-*`) under dir, leaving any non-generated content untouched. A match is
// only removed when it is a directory containing a SKILL.md — so an unexpected
// --output-dir (e.g. the repo root) cannot delete the built `slk` binary or
// unrelated files/dirs that merely match the `slk`/`slk-*` pattern.
func cleanGeneratedSkills(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "slk"))
	if err != nil {
		return err
	}
	more, err := filepath.Glob(filepath.Join(dir, "slk-*"))
	if err != nil {
		return err
	}
	for _, d := range append(matches, more...) {
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			continue // skip files (e.g. the slk binary)
		}
		if _, err := os.Stat(filepath.Join(d, "SKILL.md")); err != nil {
			continue // not a generated skill dir — leave it alone
		}
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	return nil
}
