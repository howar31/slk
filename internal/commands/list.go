package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newListCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "Create and manage Slack Lists"}
	cmd.AddCommand(
		newListCreateCommand(g),
		newListReadCommand(g),
		newListAddItemCommand(g),
		newListUpdateItemCommand(g),
	)
	return cmd
}

func newListCreateCommand(g *GlobalFlags) *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new List",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"name": title}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.create name=%q\n", title)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.create", params, nil)
			if err != nil {
				return err
			}
			var resp struct {
				List struct {
					ID string `json:"id"`
				} `json:"list"`
			}
			json.Unmarshal(raw, &resp)
			fmt.Fprintf(cmd.OutOrStdout(), "created list %s\n", resp.List.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "list title")
	cmd.MarkFlagRequired("title")
	return cmd
}

func newListReadCommand(g *GlobalFlags) *cobra.Command {
	var listID string
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read items in a List",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.items.list",
				map[string]string{"list_id": listID}, nil)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return nil
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
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"list_id": listID, "initial_fields": fieldsJSON}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.items.create list_id=%s\n", listID)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("slackLists.items.create", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "item added")
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&fieldsJSON, "fields", "[]", "initial fields as JSON array")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newListUpdateItemCommand(g *GlobalFlags) *cobra.Command {
	var listID, itemID, fieldsJSON string
	cmd := &cobra.Command{
		Use:   "update-item",
		Short: "Update an item in a List",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{
				"list_id": listID, "id": itemID, "cells": fieldsJSON,
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.items.update list_id=%s id=%s\n", listID, itemID)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("slackLists.items.update", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "item updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&itemID, "item", "", "item ID")
	cmd.Flags().StringVar(&fieldsJSON, "fields", "[]", "updated cells as JSON array")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("item")
	return cmd
}
