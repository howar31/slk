package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanvasHit_Concise(t *testing.T) {
	c := canvasHit{ID: "F1", Title: "My Canvas", Permalink: "https://x/y/F1"}
	got := c.Concise()
	if !strings.Contains(got, "My Canvas") || !strings.Contains(got, "F1") || !strings.Contains(got, "https://x/y/F1") {
		t.Fatalf("concise = %q", got)
	}
}

func TestCanvasCommand_HasListSubcommand(t *testing.T) {
	cmd := newCanvasCommand(&GlobalFlags{})
	want := map[string]bool{"create": false, "read": false, "update": false, "list": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing canvas subcommand %q", name)
		}
	}
}

func TestCanvasUpdate_ActionDryRun(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"update", "--id", "F1", "--markdown", "hi"}, "operation=replace"},
		{[]string{"update", "--id", "F1", "--markdown", "hi", "--action", "prepend"}, "operation=insert_at_start"},
		{[]string{"update", "--id", "F1", "--markdown", "hi", "--action", "append"}, "operation=insert_at_end"},
		{[]string{"update", "--id", "F1", "--markdown", "hi", "--action", "prepend", "--section-id", "S1"}, "operation=insert_before_specific_section"},
		{[]string{"update", "--id", "F1", "--markdown", "hi", "--action", "append", "--section-id", "S1"}, "operation=insert_after_specific_section"},
		{[]string{"update", "--id", "F1", "--markdown", "hi", "--action", "replace", "--section-id", "S1"}, "operation=replace"},
	} {
		g := &GlobalFlags{DryRun: true}
		cmd := newCanvasCommand(g)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("args %v: want %q in %q", tc.args, tc.want, out.String())
		}
	}
}

func TestCanvasUpdate_RejectsUnknownAction(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newCanvasCommand(g)
	cmd.SetArgs([]string{"update", "--id", "F1", "--markdown", "hi", "--action", "wat"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for unknown --action")
	}
}

func TestCanvasCreate_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newCanvasCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"create", "--title", "Plan", "--markdown", "# hi"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "canvases.create") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}

func TestCanvasUpdate_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newCanvasCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update", "--id", "F123", "--markdown", "# new"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "canvases.edit") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}

func TestCanvasUpdate_MarkdownFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.md")
	os.WriteFile(p, []byte("## H\nbody"), 0o600)
	g := &GlobalFlags{DryRun: true}
	cmd := newCanvasCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update", "--id", "F1", "--markdown-file", p, "--action", "prepend"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "operation=insert_at_start") {
		t.Fatalf("dry-run output: %q", out.String())
	}
}

func TestCanvasRead_WithSectionsFlag(t *testing.T) {
	cmd := newCanvasCommand(&GlobalFlags{})
	read, _, err := cmd.Find([]string{"read"})
	if err != nil {
		t.Fatalf("find read: %v", err)
	}
	if read.Flags().Lookup("with-sections") == nil {
		t.Fatal("missing --with-sections")
	}
}

// TestCanvasNewVerbs_FlagRegistration verifies that delete, share, and unshare
// expose exactly the required flags.
func TestCanvasNewVerbs_FlagRegistration(t *testing.T) {
	g := &GlobalFlags{}
	cases := []struct {
		verb          string
		requiredFlags []string
		optionalFlags []string
	}{
		{
			verb:          "delete",
			requiredFlags: []string{"id"},
		},
		{
			verb:          "share",
			requiredFlags: []string{"id", "access-level"},
			optionalFlags: []string{"users", "channels"},
		},
		{
			verb:          "unshare",
			requiredFlags: []string{"id"},
			optionalFlags: []string{"users", "channels"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.verb, func(t *testing.T) {
			cmd := newCanvasCommand(g)
			sub, _, err := cmd.Find([]string{tc.verb})
			if err != nil || sub == nil || sub.Name() != tc.verb {
				t.Fatalf("subcommand %q not found: %v", tc.verb, err)
			}
			for _, f := range tc.requiredFlags {
				if sub.Flags().Lookup(f) == nil {
					t.Errorf("missing required flag --%s", f)
				}
			}
			for _, f := range tc.optionalFlags {
				if sub.Flags().Lookup(f) == nil {
					t.Errorf("missing optional flag --%s", f)
				}
			}
		})
	}
}

// TestCanvasNewVerbs_DryRun verifies that each new write verb prints the
// expected Slack method name in dry-run mode and returns no error.
func TestCanvasNewVerbs_DryRun(t *testing.T) {
	cases := []struct {
		args       []string
		wantMethod string
	}{
		{
			args:       []string{"delete", "--id", "F01234567"},
			wantMethod: "canvases.delete",
		},
		{
			args:       []string{"share", "--id", "F01234567", "--access-level", "read", "--users", "U0123456789"},
			wantMethod: "canvases.access.set",
		},
		{
			args:       []string{"share", "--id", "F01234567", "--access-level", "write", "--channels", "C0123456789"},
			wantMethod: "canvases.access.set",
		},
		{
			args:       []string{"unshare", "--id", "F01234567", "--users", "U0123456789"},
			wantMethod: "canvases.access.delete",
		},
		{
			args:       []string{"unshare", "--id", "F01234567", "--channels", "C0123456789"},
			wantMethod: "canvases.access.delete",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			g := &GlobalFlags{DryRun: true}
			cmd := newCanvasCommand(g)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute %v: %v", tc.args, err)
			}
			if !strings.Contains(out.String(), tc.wantMethod) {
				t.Errorf("args %v: want method %q in output %q", tc.args, tc.wantMethod, out.String())
			}
		})
	}
}
