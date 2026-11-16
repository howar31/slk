package commands

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

// listItem is one row trimmed from slackLists.items.list.
type listItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Concise renders "<row_id>  <primary text>". When the row has no text
// fields (newly-created empty rows) only the row ID is shown.
func (it listItem) Concise() string {
	if it.Text == "" {
		return it.ID
	}
	return fmt.Sprintf("%s  %s", it.ID, it.Text)
}

// parseListItems extracts row IDs and primary text from a slackLists.items.list
// response. The primary text is the first non-empty `text` field of each row;
// this is conventionally the list's primary column.
func parseListItems(raw []byte) ([]listItem, error) {
	var resp struct {
		Items []struct {
			ID     string `json:"id"`
			Fields []struct {
				Text string `json:"text"`
			} `json:"fields"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	items := make([]listItem, 0, len(resp.Items))
	for _, it := range resp.Items {
		var text string
		for _, f := range it.Fields {
			if f.Text != "" {
				text = f.Text
				break
			}
		}
		items = append(items, listItem{ID: it.ID, Text: text})
	}
	return items, nil
}

const listFieldsExample = `JSON array of cells. Each cell needs column_id plus a typed value (rich_text for text columns). Example:
[{"column_id":"Col0…","rich_text":[{"type":"rich_text","elements":[{"type":"rich_text_section","elements":[{"type":"text","text":"hello"}]}]}]}]`

func newListCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "Create and manage Slack Lists"}
	cmd.AddCommand(
		newListCreateCommand(g),
		newListReadCommand(g),
		newListAddItemCommand(g),
		newListUpdateItemCommand(g),
		newListDeleteItemCommand(g),
		newListUpdateCommand(g),
		newListItemInfoCommand(g),
		newListDeleteItemsCommand(g),
		newListShareCommand(g),
		newListUnshareCommand(g),
		newListExportCommand(g),
	)
	return cmd
}

// listExportPollInterval is how long `list export` waits between
// slackLists.download.get calls; tests shorten it.
var listExportPollInterval = 2 * time.Second

// listExportStatus is the part of a slackLists.download.get response that
// `list export` acts on.
type listExportStatus struct {
	Status      string `json:"status"`
	DownloadURL string `json:"download_url"`
}

// waitListExport polls slackLists.download.get until the job reports
// COMPLETED with a download URL, the job reports a failure, or timeout
// passes. It returns the last raw response with the parsed status.
func waitListExport(client *api.Client, params map[string]string, timeout time.Duration) ([]byte, listExportStatus, error) {
	deadline := time.Now().Add(timeout)
	for {
		raw, err := client.Call("slackLists.download.get", params, nil)
		if err != nil {
			return nil, listExportStatus{}, err
		}
		var st listExportStatus
		if err := json.Unmarshal(raw, &st); err != nil {
			return nil, listExportStatus{}, err
		}
		status := strings.ToUpper(st.Status)
		if status == "COMPLETED" && st.DownloadURL != "" {
			return raw, st, nil
		}
		if strings.Contains(status, "FAIL") || strings.Contains(status, "ERROR") {
			return raw, st, fmt.Errorf("list export job %s: status %s", params["job_id"], st.Status)
		}
		if time.Now().Add(listExportPollInterval).After(deadline) {
			return raw, st, fmt.Errorf("list export job %s not completed after %s (last status %q)", params["job_id"], timeout, st.Status)
		}
		time.Sleep(listExportPollInterval)
	}
}

func newListExportCommand(g *GlobalFlags) *cobra.Command {
	var listID, fileFormat, out string
	var includeArchived, includeThreads, includeAttachments, force bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a List to a CSV or JSON file",
		Annotations: map[string]string{
			"slackMethod": "slackLists.download.start",
			"userScopes":  "lists:read",
			"botScopes":   "lists:read",
			"botCapable":  "true",
		},
		Long: `Starts an export job (slackLists.download.start), waits for it
(slackLists.download.get), and saves the file. --file-format is csv or json;
--include-threads and --include-attachments apply to json only. With --raw,
prints the slackLists.download.get response, which carries download_url, and
saves nothing. An existing file is kept unless --force is set.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if fileFormat != "csv" && fileFormat != "json" {
				return fmt.Errorf("--file-format must be csv or json")
			}
			if fileFormat != "json" && (includeThreads || includeAttachments) {
				return fmt.Errorf("--include-threads and --include-attachments require --file-format json")
			}
			// download.get must repeat the options passed to download.start.
			params := map[string]string{"list_id": listID, "format": fileFormat}
			if includeArchived {
				params["include_archived"] = "true"
			}
			if includeThreads {
				params["include_threads"] = "true"
			}
			if includeAttachments {
				params["include_attachments"] = "true"
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.download.start %v, then slackLists.download.get and download\n", params)
				return nil
			}
			name := listID + "." + fileFormat
			var dest string
			if !g.Raw {
				var err error
				if dest, err = downloadDest(out, name, force); err != nil {
					return err
				}
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.download.start", params, nil)
			if err != nil {
				return err
			}
			var started struct {
				JobID string `json:"job_id"`
			}
			if err := json.Unmarshal(raw, &started); err != nil {
				return err
			}
			if started.JobID == "" {
				return fmt.Errorf("slackLists.download.start returned no job_id")
			}
			getParams := map[string]string{"job_id": started.JobID}
			for k, v := range params {
				getParams[k] = v
			}
			raw, st, err := waitListExport(client, getParams, timeout)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			mimetype := "text/csv"
			if fileFormat == "json" {
				mimetype = "application/json"
			}
			if err := saveURL(client, st.DownloadURL, mimetype, "lists:read", dest); err != nil {
				return err
			}
			path, err := filepath.Abs(dest)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, []msgFile{{ID: listID, Name: name, Mimetype: mimetype, Path: path}})
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&fileFormat, "file-format", "csv", "export format: csv or json")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "include archived items")
	cmd.Flags().BoolVar(&includeThreads, "include-threads", false, "include each item's thread (json only)")
	cmd.Flags().BoolVar(&includeAttachments, "include-attachments", false, "include attachment metadata (json only)")
	cmd.Flags().StringVar(&out, "out", "", "output file or directory (default: <list ID>.<format> in the current directory)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing file")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "how long to wait for the export job")
	cmd.MarkFlagRequired("id")
	return cmd
}

// parseListRecord extracts the row from a slackLists.items.info response,
// using the same primary-text rule as parseListItems.
func parseListRecord(raw []byte) (listItem, error) {
	var resp struct {
		Record struct {
			ID     string `json:"id"`
			Fields []struct {
				Text string `json:"text"`
			} `json:"fields"`
		} `json:"record"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return listItem{}, err
	}
	it := listItem{ID: resp.Record.ID}
	for _, f := range resp.Record.Fields {
		if f.Text != "" {
			it.Text = f.Text
			break
		}
	}
	return it, nil
}

// splitIDs turns a comma-separated flag value into a trimmed, non-empty slice.
func splitIDs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func newListItemInfoCommand(g *GlobalFlags) *cobra.Command {
	var listID, rowID string
	cmd := &cobra.Command{
		Use:   "item-info",
		Short: "Read one item of a List",
		Annotations: map[string]string{
			"slackMethod": "slackLists.items.info",
			"userScopes":  "lists:read",
			"botScopes":   "lists:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"list_id": listID, "id": rowID}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.items.info", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			it, err := parseListRecord(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, []listItem{it})
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&rowID, "row-id", "", "row (item) ID")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("row-id")
	return cmd
}

func newListDeleteItemsCommand(g *GlobalFlags) *cobra.Command {
	var listID, rowIDs string
	cmd := &cobra.Command{
		Use:   "delete-items",
		Short: "Delete several items from a List",
		Annotations: map[string]string{
			"slackMethod": "slackLists.items.deleteMultiple",
			"write":       "true",
			"userScopes":  "lists:write",
			"botScopes":   "lists:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := splitIDs(rowIDs)
			if len(ids) == 0 {
				return fmt.Errorf("--row-ids must list at least one row ID")
			}
			// ids is an array argument; send a JSON body so it arrives as one.
			body, err := json.Marshal(map[string]any{"list_id": listID, "ids": ids})
			if err != nil {
				return err
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.items.deleteMultiple %s\n", body)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.items.deleteMultiple", nil, body)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d items deleted\n", len(ids))
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&rowIDs, "row-ids", "", "comma-separated row (item) IDs to delete")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("row-ids")
	return cmd
}

func newListShareCommand(g *GlobalFlags) *cobra.Command {
	var listID, accessLevel, users, channels string
	cmd := &cobra.Command{
		Use:   "share",
		Short: "Set access on a List for users or channels",
		Annotations: map[string]string{
			"slackMethod": "slackLists.access.set",
			"write":       "true",
			"userScopes":  "lists:write",
			"botScopes":   "lists:write",
			"botCapable":  "true",
		},
		Long: "Sets access on a List. --access-level is read, write, or owner; owner applies to --users only.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// slackLists.access.set accepts exactly one of user_ids / channel_ids.
			if (users == "") == (channels == "") {
				return fmt.Errorf("provide exactly one of --users or --channels")
			}
			params := map[string]string{"list_id": listID, "access_level": accessLevel}
			if users != "" {
				params["user_ids"] = users
			} else {
				params["channel_ids"] = channels
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.access.set %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.access.set", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "access set")
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&accessLevel, "access-level", "", "access level: read, write, or owner")
	cmd.Flags().StringVar(&users, "users", "", "comma-separated user IDs to grant access")
	cmd.Flags().StringVar(&channels, "channels", "", "comma-separated channel IDs to grant access")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("access-level")
	return cmd
}

func newListUnshareCommand(g *GlobalFlags) *cobra.Command {
	var listID, users, channels string
	cmd := &cobra.Command{
		Use:   "unshare",
		Short: "Remove access on a List for users or channels",
		Annotations: map[string]string{
			"slackMethod": "slackLists.access.delete",
			"write":       "true",
			"userScopes":  "lists:write",
			"botScopes":   "lists:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if (users == "") == (channels == "") {
				return fmt.Errorf("provide exactly one of --users or --channels")
			}
			params := map[string]string{"list_id": listID}
			if users != "" {
				params["user_ids"] = users
			} else {
				params["channel_ids"] = channels
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.access.delete %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.access.delete", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "access removed")
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&users, "users", "", "comma-separated user IDs to remove access")
	cmd.Flags().StringVar(&channels, "channels", "", "comma-separated channel IDs to remove access")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newListCreateCommand(g *GlobalFlags) *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new List",
		Annotations: map[string]string{
			"slackMethod": "slackLists.create",
			"write":       "true",
			"userScopes":  "lists:write",
			"botScopes":   "lists:write",
			"botCapable":  "true",
		},
		Long: "Create a Slack List. Lists cannot be deleted via the public API (slackLists.delete does not exist) — remove them in the Slack UI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"name": title}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.create name=%q\n", title)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.create", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created list %s\n", parseListCreateID(raw))
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "list title")
	cmd.MarkFlagRequired("title")
	return cmd
}

// parseListCreateID extracts the new list ID from a slackLists.create response.
// Slack returns the identifier at the top level as `list_id`.
func parseListCreateID(raw []byte) string {
	var resp struct {
		ListID string `json:"list_id"`
	}
	_ = json.Unmarshal(raw, &resp)
	return resp.ListID
}

func newListReadCommand(g *GlobalFlags) *cobra.Command {
	var listID string
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read items in a List",
		Annotations: map[string]string{
			"slackMethod": "slackLists.items.list",
			"userScopes":  "lists:read",
			"botScopes":   "lists:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.items.list",
				map[string]string{"list_id": listID}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			items, err := parseListItems(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newListAddItemCommand(g *GlobalFlags) *cobra.Command {
	var listID, fieldsJSON string
	cmd := &cobra.Command{
		Use:   "add-item",
		Short: "Add an item to a List",
		Annotations: map[string]string{
			"slackMethod": "slackLists.items.create",
			"write":       "true",
			"userScopes":  "lists:write",
			"botScopes":   "lists:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"list_id": listID, "initial_fields": fieldsJSON}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.items.create list_id=%s\n", listID)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.items.create", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "item added")
			return nil
		},
	}
	cmd.Long = "Add an item to a List.\n\n" + listFieldsExample
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&fieldsJSON, "fields", "[]", "initial fields as JSON array (see Long help for shape)")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newListUpdateItemCommand(g *GlobalFlags) *cobra.Command {
	var listID, rowID, fieldsJSON string
	cmd := &cobra.Command{
		Use:   "update-item",
		Short: "Update an item in a List",
		Annotations: map[string]string{
			"slackMethod": "slackLists.items.update",
			"write":       "true",
			"userScopes":  "lists:write",
			"botScopes":   "lists:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cells, err := injectRowID(fieldsJSON, rowID)
			if err != nil {
				return err
			}
			params := map[string]string{"list_id": listID, "cells": cells}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.items.update list_id=%s row_id=%s\n", listID, rowID)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.items.update", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "item updated")
			return nil
		},
	}
	cmd.Long = "Update an item in a List.\n\n" + listFieldsExample + `

--row-id is optional: when provided, slk injects it as the row_id of any cell
that does not already specify one. Cells with an explicit row_id keep their
own value, so the same call can update multiple rows at once.`
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&rowID, "row-id", "", "row to update; injected as cells[].row_id when a cell omits it")
	cmd.Flags().StringVar(&fieldsJSON, "fields", "[]", "updated cells as JSON array (see Long help for shape)")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newListDeleteItemCommand(g *GlobalFlags) *cobra.Command {
	var listID, rowID string
	cmd := &cobra.Command{
		Use:   "delete-item",
		Short: "Delete one item from a List",
		Annotations: map[string]string{
			"slackMethod": "slackLists.items.delete",
			"write":       "true",
			"userScopes":  "lists:write",
			"botScopes":   "lists:write",
			"botCapable":  "true",
		},
		Long: "Deletes one List item. The whole-list delete API (slackLists.delete) does not exist — remove a List in the Slack UI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"list_id": listID, "id": rowID}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.items.delete %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.items.delete", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "item deleted")
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&rowID, "row-id", "", "row (item) ID to delete")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("row-id")
	return cmd
}

func newListUpdateCommand(g *GlobalFlags) *cobra.Command {
	var listID, name, description, todoMode string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update metadata of a List",
		Annotations: map[string]string{
			"slackMethod": "slackLists.update",
			"write":       "true",
			"userScopes":  "lists:write",
			"botScopes":   "lists:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"id": listID}
			// Only include optional params that were explicitly set.
			if cmd.Flags().Changed("name") {
				params["name"] = name
			}
			if cmd.Flags().Changed("description") {
				params["description"] = description
			}
			if cmd.Flags().Changed("todo-mode") {
				params["todo_mode"] = todoMode
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.update %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.update", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "list updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&name, "name", "", "new name for the list")
	cmd.Flags().StringVar(&description, "description", "", "new description for the list")
	cmd.Flags().StringVar(&todoMode, "todo-mode", "", "todo mode string (e.g. \"on\" or \"off\")")
	cmd.MarkFlagRequired("id")
	return cmd
}

// injectRowID fills row_id on cells that omit it, using the fallback. Returns
// the cells JSON ready for slackLists.items.update. Errors when fieldsJSON is
// not a JSON array, or when both the fallback and any cell's row_id are empty.
func injectRowID(fieldsJSON, fallback string) (string, error) {
	trimmed := strings.TrimSpace(fieldsJSON)
	if trimmed == "" || trimmed == "[]" {
		if fallback == "" {
			return "", fmt.Errorf("--fields is empty and --row-id is not set; nothing to update")
		}
		// An empty cells array would no-op the update; still validate the input
		// so callers see a clear error instead of an opaque Slack response.
		return fieldsJSON, nil
	}
	var cells []map[string]any
	if err := json.Unmarshal([]byte(fieldsJSON), &cells); err != nil {
		return "", fmt.Errorf("--fields must be a JSON array of cell objects: %w", err)
	}
	for i, cell := range cells {
		existing, _ := cell["row_id"].(string)
		if existing == "" {
			if fallback == "" {
				return "", fmt.Errorf("cell %d has no row_id and --row-id is not set", i)
			}
			cell["row_id"] = fallback
			cells[i] = cell
		}
	}
	b, err := json.Marshal(cells)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
