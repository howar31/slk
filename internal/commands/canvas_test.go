package commands

import (
	"bytes"
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
