package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// newAPICommand builds `slk api <method>` — the generic escape hatch.
func newAPICommand(g *GlobalFlags) *cobra.Command {
	var paramsJSON, bodyJSON string
	cmd := &cobra.Command{
		Use:   "api <method>",
		Short: "Call any Slack Web API method directly",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := args[0]
			params := map[string]string{}
			if paramsJSON != "" {
				if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
					return fmt.Errorf("invalid --params JSON: %w", err)
				}
			}
			var body []byte
			if bodyJSON != "" {
				body = []byte(bodyJSON)
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s params=%v body=%s\n", method, params, bodyJSON)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call(method, params, body)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return nil
		},
	}
	cmd.Flags().StringVar(&paramsJSON, "params", "", "query/form params as JSON object")
	cmd.Flags().StringVar(&bodyJSON, "json", "", "request body as raw JSON")
	return cmd
}
