package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newFileCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "file", Short: "List and manage files"}
	cmd.AddCommand(
		newFileListCommand(g),
		newFileInfoCommand(g),
		newFileDownloadCommand(g),
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
		Annotations: map[string]string{
			"slackMethod": "files.list",
			"userScopes":  "files:read",
			"botScopes":   "files:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
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
		Use:   "info",
		Short: "Show file details",
		Annotations: map[string]string{
			"slackMethod": "files.info",
			"userScopes":  "files:read",
			"botScopes":   "files:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
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

// slackFile is the subset of a Slack file object slk reads, as found in
// files.info responses and in a message's files array.
type slackFile struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Mimetype           string `json:"mimetype"`
	Size               int64  `json:"size"`
	URLPrivate         string `json:"url_private"`
	URLPrivateDownload string `json:"url_private_download"`
}

// downloadURL prefers url_private_download, which Slack sets only on some files.
func (f slackFile) downloadURL() string {
	if f.URLPrivateDownload != "" {
		return f.URLPrivateDownload
	}
	return f.URLPrivate
}

func fetchFileMeta(client *api.Client, fileID string) (slackFile, error) {
	raw, err := client.Call("files.info", map[string]string{"file": fileID}, nil)
	if err != nil {
		return slackFile{}, err
	}
	var resp struct {
		File slackFile `json:"file"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return slackFile{}, err
	}
	return resp.File, nil
}

func isHTML(mimetype string) bool {
	return strings.HasPrefix(strings.ToLower(mimetype), "text/html")
}

func isImage(mimetype string) bool {
	return strings.HasPrefix(strings.ToLower(mimetype), "image/")
}

// saveFile downloads a Slack-hosted file to dest. The file appears at dest only
// once it is complete, and is readable by its owner alone because Slack files
// can be private.
func saveFile(client *api.Client, info slackFile, dest string) error {
	downloadURL := info.downloadURL()
	if downloadURL == "" {
		return fmt.Errorf("file %s has no download URL", info.ID)
	}
	resp, err := client.OpenPrivateFile(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// An HTML response to a non-HTML file is a Slack page, not the file.
	if isHTML(resp.Header.Get("Content-Type")) && !isHTML(info.Mimetype) {
		return errors.New("Slack returned an HTML page instead of the file; check that the token has files:read and can access the file")
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".slk-download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, resp.Body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return os.Rename(tmp.Name(), dest)
}

// safeFileName reduces a Slack-supplied file name to one path component, since
// the name is chosen by whoever uploaded the file. The result is empty when
// nothing usable remains.
func safeFileName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = name[strings.LastIndex(name, "/")+1:]
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	return strings.TrimLeft(name, ". ")
}

// downloadDest resolves where file download writes: --out as given, or the
// file's name inside --out when it is a directory, or in the working directory
// when --out is empty. An existing file is kept unless force is set.
func downloadDest(out, fileName string, force bool) (string, error) {
	dest := out
	if stat, err := os.Stat(out); out == "" || (err == nil && stat.IsDir()) {
		base := safeFileName(fileName)
		if base == "" {
			return "", fmt.Errorf("file name %q is not usable as a path; pass a file path with --out", fileName)
		}
		dest = filepath.Join(out, base)
	}
	if _, err := os.Stat(dest); err == nil && !force {
		return "", fmt.Errorf("%s already exists; pass --force to overwrite", dest)
	}
	return dest, nil
}

func newFileDownloadCommand(g *GlobalFlags) *cobra.Command {
	var fileID, out string
	var force bool
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download a file",
		Long: `Downloads a Slack-hosted file. Files hosted outside Slack are refused, because
the token is only sent to Slack file hosts. An existing file is kept unless
--force is set.`,
		Annotations: map[string]string{
			"slackMethod": "files.info",
			"userScopes":  "files:read",
			"botScopes":   "files:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] files.info file=%s, then download\n", fileID)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			info, err := fetchFileMeta(client, fileID)
			if err != nil {
				return err
			}
			dest, err := downloadDest(out, info.Name, force)
			if err != nil {
				return err
			}
			if err := saveFile(client, info, dest); err != nil {
				return err
			}
			path, err := filepath.Abs(dest)
			if err != nil {
				return err
			}
			saved := msgFile{ID: info.ID, Name: info.Name, Mimetype: info.Mimetype, Size: info.Size, Path: path}
			return output.Emit(cmd.OutOrStdout(), g.Format, []msgFile{saved})
		},
	}
	cmd.Flags().StringVar(&fileID, "file", "", "file ID")
	cmd.Flags().StringVar(&out, "out", "", "output file or directory (default: the file's name in the current directory)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing file")
	cmd.MarkFlagRequired("file")
	return cmd
}

// saveMessageImages saves every image attached to items into dir, recording
// the local path or the error on each file. It returns how many failed; one
// failure does not stop the rest.
func saveMessageImages(client *api.Client, items []msgItem, dir string) int {
	failed := 0
	for i := range items {
		for j := range items[i].Files {
			file := &items[i].Files[j]
			if err := saveMessageImage(client, file, dir); err != nil {
				file.SaveError = err.Error()
				failed++
			}
		}
	}
	return failed
}

// messageImagePath prefixes the file ID because names repeat: pasted
// screenshots are all called image.png.
func messageImagePath(dir, fileID, name string) (string, error) {
	return filepath.Abs(filepath.Join(dir, safeFileName(fileID)+"-"+safeFileName(name)))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// saveMessageImage skips non-images and images already in dir without calling
// Slack. A file listed without a mimetype (a Slack Connect stub) is looked up
// first, since only files.info says what it is.
func saveMessageImage(client *api.Client, file *msgFile, dir string) error {
	if file.Mimetype != "" && !isImage(file.Mimetype) {
		return nil
	}
	if file.Name != "" {
		path, err := messageImagePath(dir, file.ID, file.Name)
		if err != nil {
			return err
		}
		if fileExists(path) {
			file.Path = path
			return nil
		}
	}

	info, err := fetchFileMeta(client, file.ID)
	if err != nil {
		return err
	}
	file.Name, file.Mimetype, file.Size = info.Name, info.Mimetype, info.Size
	if !isImage(info.Mimetype) {
		return nil
	}
	path, err := messageImagePath(dir, file.ID, info.Name)
	if err != nil {
		return err
	}
	if !fileExists(path) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := saveFile(client, info, path); err != nil {
			return err
		}
	}
	file.Path = path
	return nil
}

func newFileUploadCommand(g *GlobalFlags) *cobra.Command {
	var filePath, channel, title string
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a file",
		Annotations: map[string]string{
			"slackMethod": "files.getUploadURLExternal",
			"write":       "true",
			"userScopes":  "files:write",
			"botScopes":   "files:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] files.upload file=%s\n", filePath)
				return nil
			}
			client, err := buildClient(cmd, g)
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
			"userScopes":  "files:write",
			"botScopes":   "files:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"file": fileID}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] files.delete %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
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
			"userScopes":  "files:write",
			"botScopes":   "",
			"botCapable":  "false",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"file": fileID}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] files.sharedPublicURL %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
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
			"userScopes":  "files:write",
			"botScopes":   "",
			"botCapable":  "false",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"file": fileID}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] files.revokePublicURL %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
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
