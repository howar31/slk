package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newCanvasCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "canvas", Short: "Create, read, update canvases"}
	cmd.AddCommand(newCanvasCreateCommand(g), newCanvasReadCommand(g), newCanvasUpdateCommand(g))
	return cmd
}

// canvasDocumentContent builds the document_content param for canvas methods.
func canvasDocumentContent(markdown string) string {
	b, _ := json.Marshal(map[string]string{"type": "markdown", "markdown": markdown})
	return string(b)
}

func newCanvasCreateCommand(g *GlobalFlags) *cobra.Command {
	var title, markdown string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a standalone canvas",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{
				"title":            title,
				"document_content": canvasDocumentContent(markdown),
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] canvases.create title=%q\n", title)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("canvases.create", params, nil)
			if err != nil {
				return err
			}
			var resp struct {
				CanvasID string `json:"canvas_id"`
			}
			json.Unmarshal(raw, &resp)
			fmt.Fprintf(cmd.OutOrStdout(), "created canvas %s\n", resp.CanvasID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "canvas title")
	cmd.Flags().StringVar(&markdown, "markdown", "", "canvas body in markdown")
	cmd.MarkFlagRequired("title")
	cmd.MarkFlagRequired("markdown")
	return cmd
}

func newCanvasReadCommand(g *GlobalFlags) *cobra.Command {
	var canvasID string
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read a canvas as raw API output",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("canvases.sections.lookup",
				map[string]string{"canvas_id": canvasID}, nil)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return nil
		},
	}
	cmd.Flags().StringVar(&canvasID, "id", "", "canvas ID")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newCanvasUpdateCommand(g *GlobalFlags) *cobra.Command {
	var canvasID, markdown string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Replace a canvas's content",
		RunE: func(cmd *cobra.Command, args []string) error {
			changes, _ := json.Marshal([]map[string]any{
				{"operation": "replace", "document_content": map[string]string{
					"type": "markdown", "markdown": markdown,
				}},
			})
			params := map[string]string{"canvas_id": canvasID, "changes": string(changes)}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] canvases.edit canvas_id=%s\n", canvasID)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("canvases.edit", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "canvas updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&canvasID, "id", "", "canvas ID")
	cmd.Flags().StringVar(&markdown, "markdown", "", "new canvas body in markdown")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("markdown")
	return cmd
}
