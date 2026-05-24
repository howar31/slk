package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newUsergroupCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "usergroup", Short: "Manage user groups"}
	cmd.AddCommand(
		newUsergroupListCommand(g),
		newUsergroupCreateCommand(g),
		newUsergroupUpdateCommand(g),
		newUsergroupEnableCommand(g),
		newUsergroupDisableCommand(g),
		newUsergroupUsersCommand(g),
		newUsergroupSetUsersCommand(g),
	)
	return cmd
}

// parseUsergroups extracts the usergroups array from a usergroups.list raw
// response and returns a slice of searchHit{Name:name, ID:id, Extra:handle}.
func parseUsergroups(raw []byte) ([]searchHit, error) {
	var resp struct {
		Usergroups []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Handle string `json:"handle"`
		} `json:"usergroups"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, len(resp.Usergroups))
	for i, ug := range resp.Usergroups {
		hits[i] = searchHit{Name: ug.Name, ID: ug.ID, Extra: ug.Handle}
	}
	return hits, nil
}

// parseUsergroupUsers extracts the users array from a usergroups.users.list
// raw response and returns a slice of searchHit{ID:userID}.
func parseUsergroupUsers(raw []byte) ([]searchHit, error) {
	var resp struct {
		Users []string `json:"users"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	hits := make([]searchHit, len(resp.Users))
	for i, id := range resp.Users {
		hits[i] = searchHit{ID: id}
	}
	return hits, nil
}

func newUsergroupListCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List user groups",
		Annotations: map[string]string{"slackMethod": "usergroups.list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("usergroups.list", map[string]string{}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parseUsergroups(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}

func newUsergroupCreateCommand(g *GlobalFlags) *cobra.Command {
	var name, handle, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user group",
		Annotations: map[string]string{
			"slackMethod": "usergroups.create",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"name": name}
			if handle != "" {
				params["handle"] = handle
			}
			if description != "" {
				params["description"] = description
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] usergroups.create %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("usergroups.create", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "usergroup created")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "user group name")
	cmd.Flags().StringVar(&handle, "handle", "", "user group handle (optional)")
	cmd.Flags().StringVar(&description, "description", "", "user group description (optional)")
	cmd.MarkFlagRequired("name")
	return cmd
}

func newUsergroupUpdateCommand(g *GlobalFlags) *cobra.Command {
	var usergroup, name, handle string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a user group",
		Annotations: map[string]string{
			"slackMethod": "usergroups.update",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"usergroup": usergroup}
			if name != "" {
				params["name"] = name
			}
			if handle != "" {
				params["handle"] = handle
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] usergroups.update %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("usergroups.update", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "usergroup updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&usergroup, "usergroup", "", "user group ID")
	cmd.Flags().StringVar(&name, "name", "", "new user group name (optional)")
	cmd.Flags().StringVar(&handle, "handle", "", "new user group handle (optional)")
	cmd.MarkFlagRequired("usergroup")
	return cmd
}

func newUsergroupEnableCommand(g *GlobalFlags) *cobra.Command {
	var usergroup string
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable a user group",
		Annotations: map[string]string{
			"slackMethod": "usergroups.enable",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"usergroup": usergroup}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] usergroups.enable %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("usergroups.enable", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "usergroup enabled")
			return nil
		},
	}
	cmd.Flags().StringVar(&usergroup, "usergroup", "", "user group ID")
	cmd.MarkFlagRequired("usergroup")
	return cmd
}

func newUsergroupDisableCommand(g *GlobalFlags) *cobra.Command {
	var usergroup string
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable a user group",
		Annotations: map[string]string{
			"slackMethod": "usergroups.disable",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"usergroup": usergroup}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] usergroups.disable %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("usergroups.disable", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "usergroup disabled")
			return nil
		},
	}
	cmd.Flags().StringVar(&usergroup, "usergroup", "", "user group ID")
	cmd.MarkFlagRequired("usergroup")
	return cmd
}

func newUsergroupUsersCommand(g *GlobalFlags) *cobra.Command {
	var usergroup string
	cmd := &cobra.Command{
		Use:         "users",
		Short:       "List members of a user group",
		Annotations: map[string]string{"slackMethod": "usergroups.users.list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("usergroups.users.list", map[string]string{"usergroup": usergroup}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			hits, err := parseUsergroupUsers(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	cmd.Flags().StringVar(&usergroup, "usergroup", "", "user group ID")
	cmd.MarkFlagRequired("usergroup")
	return cmd
}

func newUsergroupSetUsersCommand(g *GlobalFlags) *cobra.Command {
	var usergroup, users string
	cmd := &cobra.Command{
		Use:   "set-users",
		Short: "Set the members of a user group",
		Annotations: map[string]string{
			"slackMethod": "usergroups.users.update",
			"write":       "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"usergroup": usergroup, "users": users}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] usergroups.users.update %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("usergroups.users.update", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "usergroup members set")
			return nil
		},
	}
	cmd.Flags().StringVar(&usergroup, "usergroup", "", "user group ID")
	cmd.Flags().StringVar(&users, "users", "", "comma-separated user IDs")
	cmd.MarkFlagRequired("usergroup")
	cmd.MarkFlagRequired("users")
	return cmd
}
