package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestListCreate_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newListCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"create", "--title", "Sprint backlog"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "slackLists.create") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}

func TestParseListCreateID(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"top-level list_id", `{"ok":true,"list_id":"F123","list_metadata":{}}`, "F123"},
		{"nested list.id is wrong shape", `{"ok":true,"list":{"id":"F123"}}`, ""},
		{"missing list_id", `{"ok":true}`, ""},
		{"malformed json", `not json`, ""},
		{"empty bytes", ``, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseListCreateID([]byte(tc.raw)); got != tc.want {
				t.Fatalf("parseListCreateID(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParseListItems(t *testing.T) {
	raw := []byte(`{
        "ok":true,
        "items":[
            {"id":"R1","fields":[{"text":"first"}]},
            {"id":"R2","fields":[{"text":""},{"text":"second"}]},
            {"id":"R3","fields":[]}
        ]
    }`)
	items, err := parseListItems(raw)
	if err != nil {
		t.Fatalf("parseListItems: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].ID != "R1" || items[0].Text != "first" {
		t.Errorf("item 0 = %+v", items[0])
	}
	if items[1].Text != "second" {
		t.Errorf("item 1 should skip blank field, got %q", items[1].Text)
	}
	if items[2].Text != "" {
		t.Errorf("empty row should have empty text, got %q", items[2].Text)
	}
}

func TestListItem_Concise(t *testing.T) {
	if got := (listItem{ID: "R1", Text: "hi"}).Concise(); got != "R1  hi" {
		t.Errorf("with text = %q", got)
	}
	if got := (listItem{ID: "R2"}).Concise(); got != "R2" {
		t.Errorf("no text = %q", got)
	}
}

func TestListAddItem_LongHelpDocumentsShape(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{})
	add, _, err := cmd.Find([]string{"add-item"})
	if err != nil {
		t.Fatalf("find add-item: %v", err)
	}
	if !strings.Contains(add.Long, "column_id") || !strings.Contains(add.Long, "rich_text") {
		t.Fatalf("add-item Long does not document required shape: %q", add.Long)
	}
}

func TestListUpdateItem_LongHelpMentionsRowID(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{})
	up, _, err := cmd.Find([]string{"update-item"})
	if err != nil {
		t.Fatalf("find update-item: %v", err)
	}
	if !strings.Contains(up.Long, "row_id") {
		t.Fatalf("update-item Long must mention row_id: %q", up.Long)
	}
}

func TestListUpdateItem_RowIDFlagRegistered(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{})
	up, _, err := cmd.Find([]string{"update-item"})
	if err != nil {
		t.Fatalf("find update-item: %v", err)
	}
	if up.Flags().Lookup("row-id") == nil {
		t.Fatal("update-item should expose --row-id")
	}
	if up.Flags().Lookup("item") != nil {
		t.Fatal("legacy --item flag should be gone (no alias by design)")
	}
	// --row-id is optional now; only --id (list) stays required.
	required := map[string]bool{}
	up.Flags().VisitAll(func(f *pflag.Flag) {
		if v, _ := f.Annotations[cobra.BashCompOneRequiredFlag]; len(v) > 0 && v[0] == "true" {
			required[f.Name] = true
		}
	})
	if required["row-id"] {
		t.Fatal("--row-id must be optional")
	}
	if !required["id"] {
		t.Fatal("--id must remain required")
	}
}

func TestInjectRowID(t *testing.T) {
	t.Run("fills missing row_id", func(t *testing.T) {
		in := `[{"column_id":"C1","rich_text":[]}]`
		out, err := injectRowID(in, "R1")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !strings.Contains(out, `"row_id":"R1"`) {
			t.Fatalf("missing row_id injection: %s", out)
		}
	})
	t.Run("keeps explicit row_id and fills only the bare cells", func(t *testing.T) {
		in := `[{"row_id":"Rkeep","column_id":"C1"},{"column_id":"C2"}]`
		out, err := injectRowID(in, "Rfallback")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !strings.Contains(out, `"row_id":"Rkeep"`) {
			t.Fatalf("explicit row_id lost: %s", out)
		}
		if !strings.Contains(out, `"row_id":"Rfallback"`) {
			t.Fatalf("fallback not injected for bare cell: %s", out)
		}
	})
	t.Run("errors when neither cell row_id nor fallback is set", func(t *testing.T) {
		_, err := injectRowID(`[{"column_id":"C1"}]`, "")
		if err == nil {
			t.Fatal("expected error when row_id is missing on both")
		}
	})
	t.Run("errors when fields is not a JSON array", func(t *testing.T) {
		_, err := injectRowID(`{"not":"array"}`, "R1")
		if err == nil {
			t.Fatal("expected error for non-array fields")
		}
	})
	t.Run("empty array with fallback returns as-is", func(t *testing.T) {
		out, err := injectRowID(`[]`, "R1")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if out != `[]` {
			t.Fatalf("empty array should pass through, got %q", out)
		}
	})
}

func TestListCommand_HasSubcommands(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{})
	want := map[string]bool{
		"create": false, "read": false,
		"add-item": false, "update-item": false,
		"delete-item": false, "update": false,
	}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing list subcommand %q", name)
		}
	}
}

func TestListDeleteItem_FlagsRegistered(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"delete-item"})
	if err != nil {
		t.Fatalf("find delete-item: %v", err)
	}
	for _, name := range []string{"id", "row-id"} {
		if sub.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s on list delete-item", name)
		}
	}
	// Both --id and --row-id must be required.
	required := map[string]bool{}
	sub.Flags().VisitAll(func(f *pflag.Flag) {
		if v, _ := f.Annotations[cobra.BashCompOneRequiredFlag]; len(v) > 0 && v[0] == "true" {
			required[f.Name] = true
		}
	})
	for _, name := range []string{"id", "row-id"} {
		if !required[name] {
			t.Errorf("--%s must be required on list delete-item", name)
		}
	}
}

func TestListDeleteItem_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newListCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"delete-item", "--id", "F01234567", "--row-id", "R0123456789"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "slackLists.items.delete") {
		t.Fatalf("dry-run output missing method name: %q", got)
	}
}

func TestListDeleteItem_LongMentionsSlackUI(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"delete-item"})
	if err != nil {
		t.Fatalf("find delete-item: %v", err)
	}
	if !strings.Contains(sub.Long, "Slack UI") {
		t.Fatalf("delete-item Long must mention Slack UI: %q", sub.Long)
	}
}

func TestListUpdate_FlagsRegistered(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{})
	sub, _, err := cmd.Find([]string{"update"})
	if err != nil {
		t.Fatalf("find update: %v", err)
	}
	for _, name := range []string{"id", "name", "description", "todo-mode"} {
		if sub.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s on list update", name)
		}
	}
	// --id is required; optional flags must NOT be required.
	required := map[string]bool{}
	sub.Flags().VisitAll(func(f *pflag.Flag) {
		if v, _ := f.Annotations[cobra.BashCompOneRequiredFlag]; len(v) > 0 && v[0] == "true" {
			required[f.Name] = true
		}
	})
	if !required["id"] {
		t.Fatal("--id must be required on list update")
	}
	for _, name := range []string{"name", "description", "todo-mode"} {
		if required[name] {
			t.Errorf("--%s must be optional on list update", name)
		}
	}
}

func TestListUpdate_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newListCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update", "--id", "F01234567", "--name", "New Name"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "slackLists.update") {
		t.Fatalf("dry-run output missing method name: %q", got)
	}
}

func TestListUpdate_DryRun_OnlySetParams(t *testing.T) {
	// Only --id and --name are set; description and todo-mode must not appear.
	g := &GlobalFlags{DryRun: true}
	cmd := newListCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update", "--id", "F01234567", "--name", "Sprint"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "description") {
		t.Fatalf("dry-run must not include unset --description: %q", got)
	}
	if strings.Contains(got, "todo_mode") {
		t.Fatalf("dry-run must not include unset --todo-mode: %q", got)
	}
}
