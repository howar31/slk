package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

// canvasHit is a trimmed result from search.files filtered to canvases.
type canvasHit struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Permalink string `json:"permalink"`
}

func (c canvasHit) Concise() string {
	return fmt.Sprintf("%s (%s) %s", c.Title, c.ID, c.Permalink)
}

func newCanvasCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "canvas", Short: "Create, read, update, list canvases"}
	cmd.AddCommand(newCanvasCreateCommand(g), newCanvasReadCommand(g), newCanvasUpdateCommand(g), newCanvasListCommand(g))
	return cmd
}

// canvasDocumentContent builds the document_content param for canvas methods.
func canvasDocumentContent(markdown string) string {
	// json.Marshal cannot fail here: the input is a plain string map.
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
			_ = json.Unmarshal(raw, &resp)
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
	var canvasID, markdown, action, sectionID string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a canvas's content",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve operation from (action, sectionID).
			var operation string
			switch action {
			case "replace":
				operation = "replace"
			case "prepend":
				if sectionID != "" {
					operation = "insert_before_specific_section"
				} else {
					operation = "insert_at_start"
				}
			case "append":
				if sectionID != "" {
					operation = "insert_after_specific_section"
				} else {
					operation = "insert_at_end"
				}
			default:
				return fmt.Errorf("unknown --action %q (want replace|prepend|append)", action)
			}

			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(),
					"[dry-run] canvases.edit canvas_id=%s operation=%s section_id=%s\n",
					canvasID, operation, sectionID)
				return nil
			}

			// Build the change object.
			change := map[string]any{
				"operation": operation,
				"document_content": map[string]string{
					"type":     "markdown",
					"markdown": markdown,
				},
			}
			if sectionID != "" {
				change["section_id"] = sectionID
			}
			changes, _ := json.Marshal([]map[string]any{change})
			params := map[string]string{"canvas_id": canvasID, "changes": string(changes)}

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
	cmd.Flags().StringVar(&action, "action", "replace", "edit action: replace (default), prepend, append")
	cmd.Flags().StringVar(&sectionID, "section-id", "", "optional section ID to target")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("markdown")
	return cmd
}

func newCanvasListCommand(g *GlobalFlags) *cobra.Command {
	var userQuery string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List canvases via search.files",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			q := "type:canvases"
			if userQuery != "" {
				q = userQuery + " type:canvases"
			}
			raw, err := client.Call("search.files", map[string]string{
				"query": q,
				"count": fmt.Sprintf("%d", limit),
			}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Files struct {
					Matches []struct {
						ID        string `json:"id"`
						Title     string `json:"title"`
						Permalink string `json:"permalink"`
						User      string `json:"user"`
					} `json:"matches"`
				} `json:"files"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			hits := make([]canvasHit, len(resp.Files.Matches))
			for i, m := range resp.Files.Matches {
				hits[i] = canvasHit{ID: m.ID, Title: m.Title, Permalink: m.Permalink}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&userQuery, "query", "", "extra search terms prepended to type:canvases")
	cmd.Flags().IntVar(&limit, "limit", 20, "max results (1-100)")
	return cmd
}
