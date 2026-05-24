package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	manifestBegin = "<!-- BEGIN GENERATED MANIFEST -->"
	manifestEnd   = "<!-- END GENERATED MANIFEST -->"
)

// newGenerateManifestCommand builds the hidden `generate-manifest` command,
// which rewrites the README app-manifest region (between the BEGIN/END markers)
// from the command tree's scope annotations. Kept separate from
// generate-skills so each generated artifact has its own clearly-named command
// and CI guard.
func newGenerateManifestCommand() *cobra.Command {
	var readme string
	cmd := &cobra.Command{
		Use:    "generate-manifest",
		Short:  "Regenerate the README app-manifest scopes from the command tree",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			block, err := renderManifestBlock(cmd.Root())
			if err != nil {
				return err
			}
			data, err := os.ReadFile(readme)
			if err != nil {
				return err
			}
			s := string(data)
			i := strings.Index(s, manifestBegin)
			j := strings.Index(s, manifestEnd)
			if i < 0 || j < 0 || j < i {
				return fmt.Errorf("manifest markers not found in %s", readme)
			}
			out := s[:i] + manifestBegin + "\n" + block + "\n" + s[j:]
			return os.WriteFile(readme, []byte(out), 0o644)
		},
	}
	cmd.Flags().StringVar(&readme, "readme", "README.md", "path to README.md")
	return cmd
}

// renderManifestBlock renders the fenced JSON manifest with user + bot scope
// arrays generated from root.
func renderManifestBlock(root *cobra.Command) (string, error) {
	type scopes struct {
		User []string `json:"user"`
		Bot  []string `json:"bot"`
	}
	manifest := map[string]any{
		"display_information": map[string]any{"name": "slk"},
		"oauth_config": map[string]any{
			"scopes": scopes{User: scopeUnion(root, "user"), Bot: scopeUnion(root, "bot")},
		},
		"settings": map[string]any{
			"org_deploy_enabled":     false,
			"socket_mode_enabled":    false,
			"token_rotation_enabled": false,
		},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return "", err
	}
	return "```json\n" + strings.TrimRight(buf.String(), "\n") + "\n```", nil
}
