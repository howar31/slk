package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/howar31/slk/internal/output"
	"github.com/howar31/slk/internal/quip"
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
	var title, markdown, markdownFile string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a standalone canvas",
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readContent(markdown, markdownFile, "--markdown", "--markdown-file")
			if err != nil {
				return err
			}
			params := map[string]string{
				"title":            title,
				"document_content": canvasDocumentContent(content),
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
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
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
	cmd.Flags().StringVar(&markdownFile, "markdown-file", "", "path to markdown file (use - for stdin)")
	cmd.MarkFlagRequired("title")
	return cmd
}

func newCanvasReadCommand(g *GlobalFlags) *cobra.Command {
	var canvasID string
	var withSections bool
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read a canvas as markdown (HTML-converted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			// 1. files.info → get url_private_download
			raw, err := client.Call("files.info", map[string]string{"file": canvasID}, nil)
			if err != nil {
				return err
			}
			var info struct {
				File struct {
					URLPrivateDownload string `json:"url_private_download"`
					URLPrivate         string `json:"url_private"`
				} `json:"file"`
			}
			if err := json.Unmarshal(raw, &info); err != nil {
				return err
			}
			downloadURL := info.File.URLPrivateDownload
			if downloadURL == "" {
				downloadURL = info.File.URLPrivate
			}
			if downloadURL == "" {
				return fmt.Errorf("canvas read: no download URL on file %s", canvasID)
			}

			// 2. Authenticated GET → HTML
			req, _ := http.NewRequest("GET", downloadURL, nil)
			req.Header.Set("Authorization", "Bearer "+client.Token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if resp.StatusCode != 200 {
				return fmt.Errorf("canvas read: download returned %d", resp.StatusCode)
			}

			// 3. Output
			if g.Raw {
				fmt.Fprint(cmd.OutOrStdout(), string(body))
				return nil
			}
			md, sections, err := quip.Convert(string(body))
			if err != nil {
				return err
			}
			if withSections {
				out := struct {
					Markdown string            `json:"markdown"`
					Sections map[string]string `json:"sections"`
				}{md, sections}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			fmt.Fprint(cmd.OutOrStdout(), md)
			return nil
		},
	}
	cmd.Flags().StringVar(&canvasID, "id", "", "canvas ID")
	cmd.Flags().BoolVar(&withSections, "with-sections", false, "also emit the section_id mapping (JSON output)")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newCanvasUpdateCommand(g *GlobalFlags) *cobra.Command {
	var canvasID, markdown, markdownFile, action, sectionID string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a canvas's content",
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := readContent(markdown, markdownFile, "--markdown", "--markdown-file")
			if err != nil {
				return err
			}

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
					"markdown": content,
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
			raw, err := client.Call("canvases.edit", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "canvas updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&canvasID, "id", "", "canvas ID")
	cmd.Flags().StringVar(&markdown, "markdown", "", "new canvas body in markdown")
	cmd.Flags().StringVar(&markdownFile, "markdown-file", "", "path to markdown file (use - for stdin)")
	cmd.Flags().StringVar(&action, "action", "replace", "edit action: replace (default), prepend, append")
	cmd.Flags().StringVar(&sectionID, "section-id", "", "optional section ID to target")
	cmd.MarkFlagRequired("id")
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
