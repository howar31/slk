package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

// slackbotSkill is one skill from slackbot.skills.list / slackbot.skills.info.
// Instructions is only filled by slackbot.skills.info.
type slackbotSkill struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description,omitempty"`
	Category     string `json:"category,omitempty"`
	Access       string `json:"access,omitempty"`
	IsOfficial   bool   `json:"is_official"`
	Permalink    string `json:"permalink,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// Concise renders "<display name> (<id>)", the category when set, and the
// instructions below a blank line when present.
func (s slackbotSkill) Concise() string {
	line := fmt.Sprintf("%s (%s)", s.DisplayName, s.ID)
	if s.Category != "" {
		line += " — " + s.Category
	}
	if s.Instructions != "" {
		line += "\n\n" + s.Instructions
	}
	return line
}

func parseSlackbotSkills(raw []byte) ([]slackbotSkill, error) {
	var resp struct {
		Skills []slackbotSkill `json:"skills"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp.Skills, nil
}

func parseSlackbotSkill(raw []byte) (slackbotSkill, error) {
	var resp struct {
		Skill slackbotSkill `json:"skill"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return slackbotSkill{}, err
	}
	return resp.Skill, nil
}

const slackbotSkillCategories = "communication, customer_support, data_and_analytics, engineering, product_management, productivity, professional_development, project_management, sales, system, team_management, other"

func newSlackbotSkillCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "slackbot-skill",
		Short: "Manage Slackbot skills",
		Long: `Manages Slackbot skills as the token's owner. With a bot token the caller is
the bot: list, enable, and disable act on the bot's own library, create adds to
it, and the bot has only the file access granted to the bot itself.`,
	}
	cmd.AddCommand(
		newSlackbotSkillListCommand(g),
		newSlackbotSkillInfoCommand(g),
		newSlackbotSkillCreateCommand(g),
		newSlackbotSkillUpdateCommand(g),
		newSlackbotSkillDeleteCommand(g),
		newSlackbotSkillToggleCommand(g, "enable", "slackbot.skills.library.add",
			"Turn a skill on in the caller's Slackbot library", "skill enabled"),
		newSlackbotSkillToggleCommand(g, "disable", "slackbot.skills.library.remove",
			"Turn a skill off in the caller's Slackbot library", "skill disabled"),
	)
	return cmd
}

func newSlackbotSkillListCommand(g *GlobalFlags) *cobra.Command {
	var filter, category string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Slackbot skills",
		Annotations: map[string]string{
			"slackMethod": "slackbot.skills.list",
			"userScopes":  "skills:read",
			"botScopes":   "skills:read",
			"botCapable":  "true",
		},
		Long: `Lists skills from the caller's point of view. --filter is library (enabled
for the caller, default), owned (created by the caller), or catalog (the
organization's official skills). Instructions are not included; use
slackbot-skill info.`,
		// --raw is not offered here: a multi-page response has no single raw envelope.
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"filter": filter}
			if category != "" {
				params["category"] = category
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			pages, err := client.CallAll("slackbot.skills.list", params, 10)
			if err != nil {
				return err
			}
			var skills []slackbotSkill
			for _, page := range pages {
				s, err := parseSlackbotSkills(page)
				if err != nil {
					return err
				}
				skills = append(skills, s...)
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, skills)
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "library", "library, owned, or catalog")
	cmd.Flags().StringVar(&category, "category", "", "only skills in this category")
	return cmd
}

func newSlackbotSkillInfoCommand(g *GlobalFlags) *cobra.Command {
	var skillID string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Read one Slackbot skill with its instructions",
		Annotations: map[string]string{
			"slackMethod": "slackbot.skills.info",
			"userScopes":  "skills:read",
			"botScopes":   "skills:read",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackbot.skills.info", map[string]string{"skill_id": skillID}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			s, err := parseSlackbotSkill(raw)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, []slackbotSkill{s})
		},
	}
	cmd.Flags().StringVar(&skillID, "id", "", "skill ID (a canvas file ID or a builtin: reference)")
	cmd.MarkFlagRequired("id")
	return cmd
}

// mcpServersJSON validates --mcp-servers as a JSON array.
func mcpServersJSON(s string) (json.RawMessage, error) {
	var arr []any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, fmt.Errorf("--mcp-servers must be a JSON array: %w", err)
	}
	return json.RawMessage(s), nil
}

func newSlackbotSkillCreateCommand(g *GlobalFlags) *cobra.Command {
	var name, instructions, instructionsFile, canvasID, description, category, examplePrompt, mcpServers string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a Slackbot skill from markdown or from a canvas",
		Annotations: map[string]string{
			"slackMethod": "slackbot.skills.create",
			"write":       "true",
			"userScopes":  "skills:write",
			"botScopes":   "skills:write",
			"botCapable":  "true",
		},
		Long: `Creates a skill from markdown (--instructions or --instructions-file) or
from a canvas the caller owns (--canvas); pass exactly one. The skill is added
to the caller's own Slackbot library. --category is one of: ` + slackbotSkillCategories + `.
--mcp-servers is a JSON array of the apps the skill needs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"display_name": name}
			if canvasID != "" {
				if instructions != "" || instructionsFile != "" {
					return fmt.Errorf("pass --canvas or --instructions/--instructions-file, not both")
				}
				body["canvas_id"] = canvasID
			} else {
				content, err := readContent(instructions, instructionsFile, "--instructions", "--instructions-file")
				if err != nil {
					return fmt.Errorf("%w (or pass --canvas)", err)
				}
				body["instructions"] = content
			}
			if description != "" {
				body["description"] = description
			}
			if category != "" {
				body["category"] = category
			}
			if examplePrompt != "" {
				body["example_prompt"] = examplePrompt
			}
			if mcpServers != "" {
				m, err := mcpServersJSON(mcpServers)
				if err != nil {
					return err
				}
				body["mcp_servers"] = m
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackbot.skills.create display_name=%q canvas_id=%q\n", name, canvasID)
				return nil
			}
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackbot.skills.create", nil, b)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			s, err := parseSlackbotSkill(raw)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created skill %s\n", s.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name (max 255 characters)")
	cmd.Flags().StringVar(&instructions, "instructions", "", "skill instructions in markdown")
	cmd.Flags().StringVar(&instructionsFile, "instructions-file", "", "path to a markdown file (use - for stdin)")
	cmd.Flags().StringVar(&canvasID, "canvas", "", "ID of a canvas you own to turn into a skill")
	cmd.Flags().StringVar(&description, "description", "", "when Slackbot should use the skill (max 400 characters)")
	cmd.Flags().StringVar(&category, "category", "", "skill category")
	cmd.Flags().StringVar(&examplePrompt, "example-prompt", "", "example prompt (max 200 characters; {skill} becomes the skill mention)")
	cmd.Flags().StringVar(&mcpServers, "mcp-servers", "", "JSON array of apps the skill needs")
	cmd.MarkFlagRequired("name")
	return cmd
}

func newSlackbotSkillUpdateCommand(g *GlobalFlags) *cobra.Command {
	var skillID, name, instructions, instructionsFile, description, category, examplePrompt, mcpServers string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a Slackbot skill's instructions or metadata",
		Annotations: map[string]string{
			"slackMethod": "slackbot.skills.update",
			"write":       "true",
			"userScopes":  "skills:write",
			"botScopes":   "skills:write",
			"botCapable":  "true",
		},
		Long: `Changes only the fields passed. Instructions replace the whole document;
--mcp-servers replaces the whole list ('[]' clears it). An empty
--description, --category, or --example-prompt clears that field.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"skill_id": skillID}
			f := cmd.Flags()
			if f.Changed("name") {
				body["display_name"] = name
			}
			if f.Changed("instructions") || f.Changed("instructions-file") {
				content, err := readContent(instructions, instructionsFile, "--instructions", "--instructions-file")
				if err != nil {
					return err
				}
				body["instructions"] = content
			}
			if f.Changed("description") {
				body["description"] = description
			}
			if f.Changed("category") {
				body["category"] = category
			}
			if f.Changed("example-prompt") {
				body["example_prompt"] = examplePrompt
			}
			if f.Changed("mcp-servers") {
				m, err := mcpServersJSON(mcpServers)
				if err != nil {
					return err
				}
				body["mcp_servers"] = m
			}
			if len(body) == 1 {
				return fmt.Errorf("nothing to update; pass at least one field flag")
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackbot.skills.update skill_id=%s fields=%d\n", skillID, len(body)-1)
				return nil
			}
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackbot.skills.update", nil, b)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "skill updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&skillID, "id", "", "skill ID")
	cmd.Flags().StringVar(&name, "name", "", "new display name")
	cmd.Flags().StringVar(&instructions, "instructions", "", "new instructions in markdown (replaces the whole document)")
	cmd.Flags().StringVar(&instructionsFile, "instructions-file", "", "path to a markdown file (use - for stdin)")
	cmd.Flags().StringVar(&description, "description", "", "new description (empty clears)")
	cmd.Flags().StringVar(&category, "category", "", "new category (empty clears)")
	cmd.Flags().StringVar(&examplePrompt, "example-prompt", "", "new example prompt (empty clears)")
	cmd.Flags().StringVar(&mcpServers, "mcp-servers", "", "JSON array of apps the skill needs (replaces the list)")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newSlackbotSkillDeleteCommand(g *GlobalFlags) *cobra.Command {
	var skillID string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a Slackbot skill and its canvas",
		Annotations: map[string]string{
			"slackMethod": "slackbot.skills.delete",
			"write":       "true",
			"userScopes":  "skills:write",
			"botScopes":   "skills:write",
			"botCapable":  "true",
		},
		Long: `Deletes the skill and its underlying canvas, and removes it from every
library, skill set, and the org catalog. To turn a skill off only for
yourself, use slackbot-skill disable.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"skill_id": skillID}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackbot.skills.delete %v\n", params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackbot.skills.delete", params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "skill deleted")
			return nil
		},
	}
	cmd.Flags().StringVar(&skillID, "id", "", "skill ID")
	cmd.MarkFlagRequired("id")
	return cmd
}

// newSlackbotSkillToggleCommand builds enable / disable, which differ only in
// method and wording.
func newSlackbotSkillToggleCommand(g *GlobalFlags, use, method, short, done string) *cobra.Command {
	var skillID string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Annotations: map[string]string{
			"slackMethod": method,
			"write":       "true",
			"userScopes":  "skills:write",
			"botScopes":   "skills:write",
			"botCapable":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"skill_id": skillID}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s %v\n", method, params)
				return nil
			}
			client, err := buildClient(cmd, g)
			if err != nil {
				return err
			}
			raw, err := client.Call(method, params, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), done)
			return nil
		},
	}
	cmd.Flags().StringVar(&skillID, "id", "", "skill ID")
	cmd.MarkFlagRequired("id")
	return cmd
}
