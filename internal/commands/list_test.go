package commands

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/howar31/slk/internal/api"
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
		"item-info": false, "delete-items": false,
		"share": false, "unshare": false,
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

func TestListNewVerbs_FlagRegistration(t *testing.T) {
	cases := []struct {
		verb     string
		required []string
		optional []string
	}{
		{"item-info", []string{"id", "row-id"}, nil},
		{"delete-items", []string{"id", "row-ids"}, nil},
		{"share", []string{"id", "access-level"}, []string{"users", "channels"}},
		{"unshare", []string{"id"}, []string{"users", "channels"}},
	}
	for _, tc := range cases {
		t.Run(tc.verb, func(t *testing.T) {
			sub, _, err := newListCommand(&GlobalFlags{}).Find([]string{tc.verb})
			if err != nil || sub.Name() != tc.verb {
				t.Fatalf("subcommand %q not found: %v", tc.verb, err)
			}
			for _, f := range tc.required {
				fl := sub.Flags().Lookup(f)
				if fl == nil {
					t.Errorf("missing flag --%s", f)
					continue
				}
				if fl.Annotations[cobra.BashCompOneRequiredFlag] == nil {
					t.Errorf("--%s should be required", f)
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

func TestListNewVerbs_DryRun(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"delete-items", "--id", "F01234567", "--row-ids", "Rec1, Rec2,"},
			[]string{"slackLists.items.deleteMultiple", `"ids":["Rec1","Rec2"]`, `"list_id":"F01234567"`}},
		{[]string{"share", "--id", "F01234567", "--access-level", "read", "--users", "U0123456789"},
			[]string{"slackLists.access.set", "user_ids:U0123456789", "list_id:F01234567"}},
		{[]string{"share", "--id", "F01234567", "--access-level", "write", "--channels", "C0123456789"},
			[]string{"slackLists.access.set", "channel_ids:C0123456789"}},
		{[]string{"unshare", "--id", "F01234567", "--channels", "C0123456789"},
			[]string{"slackLists.access.delete", "channel_ids:C0123456789"}},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			cmd := newListCommand(&GlobalFlags{DryRun: true})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			for _, w := range tc.want {
				if !strings.Contains(out.String(), w) {
					t.Errorf("output %q missing %q", out.String(), w)
				}
			}
		})
	}
}

func TestListDeleteItems_RejectsEmptyIDs(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{DryRun: true})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"delete-items", "--id", "F01234567", "--row-ids", " , "})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an empty --row-ids list")
	}
}

func TestListShareUnshare_RequireExactlyOneTarget(t *testing.T) {
	cases := [][]string{
		{"share", "--id", "F01234567", "--access-level", "read"},
		{"share", "--id", "F01234567", "--access-level", "read", "--users", "U0123456789", "--channels", "C0123456789"},
		{"unshare", "--id", "F01234567"},
		{"unshare", "--id", "F01234567", "--users", "U0123456789", "--channels", "C0123456789"},
	}
	for _, args := range cases {
		cmd := newListCommand(&GlobalFlags{DryRun: true})
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("%v: expected an error when not exactly one of --users/--channels", args)
		}
	}
}

func TestParseListRecord(t *testing.T) {
	raw := []byte(`{"ok":true,"list":{"id":"F01234567"},"record":{"id":"Rec1","list_id":"F01234567","fields":[{"text":""},{"text":"hello"}]},"subtasks":[]}`)
	it, err := parseListRecord(raw)
	if err != nil {
		t.Fatalf("parseListRecord: %v", err)
	}
	if it.ID != "Rec1" || it.Text != "hello" {
		t.Errorf("got %+v", it)
	}
	if _, err := parseListRecord([]byte("not json")); err == nil {
		t.Error("expected an error for malformed JSON")
	}
}

// fakeListExport serves slackLists.download.get with the given statuses in
// order (the last one repeats) and the export file at /files-pri/export.
func fakeListExport(t *testing.T, statuses ...string) (*api.Client, *atomic.Int32) {
	t.Helper()
	var gets atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slackLists.download.get":
			_ = r.ParseForm()
			if r.Form.Get("job_id") != "LeJob1" || r.Form.Get("format") != "csv" {
				fmt.Fprint(w, `{"ok":false,"error":"invalid_arguments"}`)
				return
			}
			n := int(gets.Add(1)) - 1
			if n >= len(statuses) {
				n = len(statuses) - 1
			}
			url := ""
			if statuses[n] == "COMPLETED" {
				url = "https://" + r.Host + "/files-pri/export"
			}
			fmt.Fprintf(w, `{"ok":true,"status":%q,"download_url":%q}`, statuses[n], url)
		case "/files-pri/export":
			w.Header().Set("Content-Type", "text/csv")
			fmt.Fprint(w, "Title\nfirst\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()
	c.FileHosts = []string{"127.0.0.1"}
	return c, &gets
}

func shortPoll(t *testing.T) {
	old := listExportPollInterval
	listExportPollInterval = time.Millisecond
	t.Cleanup(func() { listExportPollInterval = old })
}

func TestWaitListExport_PollsUntilCompleted(t *testing.T) {
	shortPoll(t)
	c, gets := fakeListExport(t, "IN_PROGRESS", "IN_PROGRESS", "COMPLETED")
	params := map[string]string{"list_id": "F01234567", "job_id": "LeJob1", "format": "csv"}
	_, st, err := waitListExport(c, params, time.Second)
	if err != nil {
		t.Fatalf("waitListExport: %v", err)
	}
	if gets.Load() != 3 || !strings.HasSuffix(st.DownloadURL, "/files-pri/export") {
		t.Errorf("gets=%d status=%+v", gets.Load(), st)
	}

	dest := filepath.Join(t.TempDir(), "out.csv")
	if err := saveURL(c, st.DownloadURL, "text/csv", "lists:read", dest); err != nil {
		t.Fatalf("saveURL: %v", err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "Title\nfirst\n" {
		t.Errorf("saved %q", b)
	}
}

func TestWaitListExport_Failed(t *testing.T) {
	shortPoll(t)
	c, _ := fakeListExport(t, "FAILED")
	params := map[string]string{"list_id": "F01234567", "job_id": "LeJob1", "format": "csv"}
	if _, _, err := waitListExport(c, params, time.Second); err == nil {
		t.Fatal("expected an error for a failed job")
	}
}

func TestWaitListExport_Timeout(t *testing.T) {
	shortPoll(t)
	c, _ := fakeListExport(t, "IN_PROGRESS")
	params := map[string]string{"list_id": "F01234567", "job_id": "LeJob1", "format": "csv"}
	if _, _, err := waitListExport(c, params, 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "not completed") {
		t.Fatalf("want a timeout error, got %v", err)
	}
}

func TestListExport_DryRunAndValidation(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{DryRun: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"export", "--id", "F01234567", "--file-format", "json", "--include-threads"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, w := range []string{"slackLists.download.start", "format:json", "include_threads:true"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("dry-run output %q missing %q", out.String(), w)
		}
	}

	bad := [][]string{
		{"export", "--id", "F01234567", "--file-format", "xlsx"},
		{"export", "--id", "F01234567", "--include-threads"},
		{"export", "--id", "F01234567", "--include-attachments"},
	}
	for _, args := range bad {
		cmd := newListCommand(&GlobalFlags{DryRun: true})
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("%v: expected a validation error", args)
		}
	}
}
