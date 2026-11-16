package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSlackbotSkill_FlagRegistration(t *testing.T) {
	cases := []struct {
		verb     string
		required []string
		optional []string
	}{
		{"list", nil, []string{"filter", "category"}},
		{"info", []string{"id"}, nil},
		{"create", []string{"name"}, []string{"instructions", "instructions-file", "canvas", "description", "category", "example-prompt", "mcp-servers"}},
		{"update", []string{"id"}, []string{"name", "instructions", "instructions-file", "description", "category", "example-prompt", "mcp-servers"}},
		{"delete", []string{"id"}, nil},
		{"enable", []string{"id"}, nil},
		{"disable", []string{"id"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.verb, func(t *testing.T) {
			sub, _, err := newSlackbotSkillCommand(&GlobalFlags{}).Find([]string{tc.verb})
			if err != nil || sub.Name() != tc.verb {
				t.Fatalf("subcommand %q not found: %v", tc.verb, err)
			}
			for _, f := range tc.required {
				fl := sub.Flags().Lookup(f)
				if fl == nil || fl.Annotations[cobra.BashCompOneRequiredFlag] == nil {
					t.Errorf("--%s missing or not required", f)
				}
			}
			for _, f := range tc.optional {
				if sub.Flags().Lookup(f) == nil {
					t.Errorf("missing flag --%s", f)
				}
			}
		})
	}
}

func TestSlackbotSkill_DryRun(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"create", "--name", "Daily", "--instructions", "# Purpose"}, "slackbot.skills.create"},
		{[]string{"create", "--name", "Daily", "--canvas", "F01234567"}, `canvas_id="F01234567"`},
		{[]string{"update", "--id", "F01234567", "--description", ""}, "slackbot.skills.update skill_id=F01234567 fields=1"},
		{[]string{"update", "--id", "F01234567", "--mcp-servers", "[]"}, "fields=1"},
		{[]string{"delete", "--id", "F01234567"}, "slackbot.skills.delete"},
		{[]string{"enable", "--id", "F01234567"}, "slackbot.skills.library.add"},
		{[]string{"disable", "--id", "F01234567"}, "slackbot.skills.library.remove"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			cmd := newSlackbotSkillCommand(&GlobalFlags{DryRun: true})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("output %q missing %q", out.String(), tc.want)
			}
		})
	}
}

func TestSlackbotSkill_Validation(t *testing.T) {
	cases := [][]string{
		{"create", "--name", "Daily"},
		{"create", "--name", "Daily", "--canvas", "F01234567", "--instructions", "x"},
		{"create", "--name", "Daily", "--instructions", "x", "--mcp-servers", "{}"},
		{"update", "--id", "F01234567"},
		{"update", "--id", "F01234567", "--mcp-servers", "nope"},
	}
	for _, args := range cases {
		cmd := newSlackbotSkillCommand(&GlobalFlags{DryRun: true})
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("%v: expected a validation error", args)
		}
	}
}

func TestParseSlackbotSkills(t *testing.T) {
	raw := []byte(`{"ok":true,"skills":[{"id":"F01234567","type":"canvas","display_name":"Daily","name":null,"category":"productivity","instructions":null,"is_official":true}],"response_metadata":{"next_cursor":""}}`)
	skills, err := parseSlackbotSkills(raw)
	if err != nil || len(skills) != 1 {
		t.Fatalf("got %v, %v", skills, err)
	}
	if got := skills[0].Concise(); got != "Daily (F01234567) — productivity" {
		t.Errorf("Concise = %q", got)
	}

	s, err := parseSlackbotSkill([]byte(`{"ok":true,"skill":{"id":"F01234567","display_name":"Daily","instructions":"# Purpose"}}`))
	if err != nil {
		t.Fatalf("parseSlackbotSkill: %v", err)
	}
	if got := s.Concise(); got != "Daily (F01234567)\n\n# Purpose" {
		t.Errorf("Concise = %q", got)
	}
}

func TestParseTeamBilling(t *testing.T) {
	cases := map[string]string{"free": "Free", "std": "Pro", "plus": "Business+", "newplan": "newplan"}
	for plan, want := range cases {
		hit, err := parseTeamBilling([]byte(`{"ok":true,"plan":"` + plan + `"}`))
		if err != nil || hit.Name != want || hit.ID != plan {
			t.Errorf("plan %q: got %+v, %v", plan, hit, err)
		}
	}
}
