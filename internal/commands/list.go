package commands

import (
	"encoding/json"
	"fmt"
	"strings"

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
	)
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
		Use:         "read",
		Short:       "Read items in a List",
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
