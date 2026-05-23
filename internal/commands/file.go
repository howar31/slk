package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newFileCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "file", Short: "List and manage files"}
	cmd.AddCommand(
		newFileListCommand(g),
		newFileInfoCommand(g),
		newFileUploadCommand(g),
		newFileDeleteCommand(g),
		newFilePublicCommand(g),
		newFileRevokePublicCommand(g),
	)
	return cmd
}

// newFileListCommand returns a command that pages through files.list and
// returns trimmed file hits. --raw is not offered here: a multi-page response
// has no single raw envelope.
func newFileListCommand(g *GlobalFlags) *cobra.Command {
	var channel, user, types string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List files",
		// --raw is not offered here: a multi-page response has no single raw envelope.
		Annotations: map[string]string{"slackMethod": "files.list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			hits, err := fileFetchList(client, channel, user, types)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "filter by channel ID (optional)")
	cmd.Flags().StringVar(&user, "user", "", "filter by user ID (optional)")
	cmd.Flags().StringVar(&types, "types", "", "filter by file types, comma-separated (optional)")
	return cmd
}

// fileFetchList pages through files.list (page-based) and returns trimmed hits.
// Stops after 10 pages.
func fileFetchList(client interface {
	Call(string, map[string]string, []byte) ([]byte, error)
}, channel, user, types string) ([]searchHit, error) {
	params := map[string]string{"page": "1"}
	if channel != "" {
		params["channel"] = channel
	}
	if user != "" {
		params["user"] = user
	}
	if types != "" {
		params["types"] = types
	}

	var hits []searchHit
	const maxPages = 10
	for page := 1; page <= maxPages; page++ {
		params["page"] = fmt.Sprint(page)
		raw, err := client.Call("files.list", params, nil)
		if err != nil {
			return hits, err
		}
		var resp struct {
			Files []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Filetype string `json:"filetype"`
			} `json:"files"`
			Paging struct {
				Page  int `json:"page"`
				Pages int `json:"pages"`
			} `json:"paging"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return hits, err
		}
		for _, f := range resp.Files {
			hits = append(hits, searchHit{Name: f.Name, ID: f.ID, Extra: f.Filetype})
		}
		if resp.Paging.Page >= resp.Paging.Pages {
			break
		}
	}
	return hits, nil
}

// parseFileInfo extracts the file object from a files.info raw response and
// returns a single searchHit.
func parseFileInfo(raw []byte) (searchHit, error) {
	var resp struct {
		File struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Filetype string `json:"filetype"`
		} `json:"file"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return searchHit{}, err
	}
	return searchHit{Name: resp.File.Name, ID: resp.File.ID, Extra: resp.File.Filetype}, nil
}

func newFileInfoCommand(g *GlobalFlags) *cobra.Command {
	var fileID string
	cmd := &cobra.Command{
		Use:         "info",
		Short:       "Show file details",
		Annotations: map[string]string{"slackMethod": "files.info"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("files.info", map[string]string{"file": fileID}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hit, err := parseFileInfo(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, []searchHit{hit})
		},
	}
	cmd.Flags().StringVar(&fileID, "file", "", "file ID")
	cmd.MarkFlagRequired("file")
	return cmd
}

func newFileUploadCommand(g *GlobalFlags) *cobra.Command {
	var filePath, channel, title string
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a file",
		Annotations: map[string]string{
			"slackMethod": "files.getUploadURLExternal",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] files.upload file=%s\n", filePath)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}

			// Step 1: stat the file (for length) and request an upload URL.
			stat, err := os.Stat(filePath)
			if err != nil {
				return fmt.Errorf("stating file: %w", err)
			}
			filename := filepath.Base(filePath)
			raw, err := client.Call("files.getUploadURLExternal", map[string]string{
				"filename": filename,
				"length":   fmt.Sprint(stat.Size()),
			}, nil)
			if err != nil {
				return err
			}

			var urlResp struct {
				UploadURL string `json:"upload_url"`
				FileID    string `json:"file_id"`
			}
			if err := json.Unmarshal(raw, &urlResp); err != nil {
				return fmt.Errorf("parsing upload URL response: %w", err)
			}

			// Step 2: stream the file to the upload URL via the client's HTTP
			// config (timeouts, proxy). An *os.File body lets net/http set
			// Content-Length without buffering the whole file in memory.
			f, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("opening file: %w", err)
			}
			defer f.Close()
			uploadResp, err := client.HTTP.Post(urlResp.UploadURL, "application/octet-stream", f)
			if err != nil {
				return fmt.Errorf("uploading file content: %w", err)
			}
			uploadResp.Body.Close()
			if uploadResp.StatusCode < 200 || uploadResp.StatusCode >= 300 {
				return fmt.Errorf("upload returned non-2xx status: %d", uploadResp.StatusCode)
			}

			// Step 3: complete the upload.
			fileTitle := title
			if fileTitle == "" {
				fileTitle = filename
			}
			filesArg, err := json.Marshal([]map[string]string{
				{"id": urlResp.FileID, "title": fileTitle},
			})
			if err != nil {
				return fmt.Errorf("encoding files parameter: %w", err)
			}
			completeParams := map[string]string{"files": string(filesArg)}
			if channel != "" {
				completeParams["channel_id"] = channel
			}
			raw, err = client.Call("files.completeUploadExternal", completeParams, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "uploaded %s\n", urlResp.FileID)
			return nil
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "path to file to upload")
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID to share the file into (optional)")
	cmd.Flags().StringVar(&title, "title", "", "file title (defaults to filename)")
	cmd.MarkFlagRequired("file")
	return cmd
}

func newFileDeleteCommand(g *GlobalFlags) *cobra.Command {
	var fileID string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a file",
		Annotations: map[string]string{
			"slackMethod": "files.delete",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"file": fileID}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] files.delete %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("files.delete", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "deleted")
			return nil
		},
	}
	cmd.Flags().StringVar(&fileID, "file", "", "file ID")
	cmd.MarkFlagRequired("file")
	return cmd
}

func newFilePublicCommand(g *GlobalFlags) *cobra.Command {
	var fileID string
	cmd := &cobra.Command{
		Use:   "public",
		Short: "Make a file publicly accessible",
		Long:  "Makes the file accessible to anyone with the link.",
		Annotations: map[string]string{
			"slackMethod": "files.sharedPublicURL",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"file": fileID}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] files.sharedPublicURL %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("files.sharedPublicURL", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "made public")
			return nil
		},
	}
	cmd.Flags().StringVar(&fileID, "file", "", "file ID")
	cmd.MarkFlagRequired("file")
	return cmd
}

func newFileRevokePublicCommand(g *GlobalFlags) *cobra.Command {
	var fileID string
	cmd := &cobra.Command{
		Use:   "revoke-public",
		Short: "Revoke a file's public link",
		Annotations: map[string]string{
			"slackMethod": "files.revokePublicURL",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"file": fileID}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] files.revokePublicURL %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("files.revokePublicURL", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "public access revoked")
			return nil
		},
	}
	cmd.Flags().StringVar(&fileID, "file", "", "file ID")
	cmd.MarkFlagRequired("file")
	return cmd
}
