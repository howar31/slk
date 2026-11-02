package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestThreadRead_FlagsRegistered(t *testing.T) {
	g := &GlobalFlags{}
	cmd := newThreadCommand(g)
	read, _, err := cmd.Find([]string{"read"})
	if err != nil {
		t.Fatalf("find read: %v", err)
	}
	for _, name := range []string{"oldest", "latest", "cursor", "limit", "channel", "thread"} {
		if read.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s on thread read", name)
		}
	}
	if read.Flags().Lookup("ts") != nil {
		t.Error("--ts must be removed; both thread verbs use --thread (no alias)")
	}
}

func TestThreadReply_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newThreadCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"reply", "--channel", "C1", "--thread", "1.0", "--text", "reply!"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "reply!") {
		t.Fatalf("dry-run output = %q: missing text 'reply!'", got)
	}
	if !strings.Contains(got, "C1") {
		t.Fatalf("dry-run output = %q: missing channel 'C1'", got)
	}
	if !strings.Contains(got, "thread_ts") {
		t.Fatalf("dry-run output = %q: missing 'thread_ts' param", got)
	}
}

func TestThreadTSFromRef(t *testing.T) {
	cases := []struct {
		name    string
		ref     messageRef
		want    string
		wantErr bool
	}{
		{"parent link", messageRef{Channel: "C0123456789", TS: "1700000000.000100"}, "1700000000.000100", false},
		{"reply link", messageRef{Channel: "C0123456789", TS: "1700000000.123456", ThreadTS: "1700000000.000100"}, "1700000000.000100", false},
		{"channel link", messageRef{Channel: "C0123456789"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := threadTSFromRef(tc.ref)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestThreadRead_FlagValidation(t *testing.T) {
	const messageURL = "https://acme.slack.com/archives/C0123456789/p1700000000123456"
	cases := []struct {
		name    string
		global  GlobalFlags
		args    []string
		wantErr string
	}{
		{"no target", GlobalFlags{}, []string{"read"}, "channel"},
		{"channel without thread", GlobalFlags{}, []string{"read", "--channel", "C0123456789"}, "thread"},
		{"url and channel", GlobalFlags{}, []string{"read", "--url", messageURL, "--channel", "C0123456789", "--thread", "1.0"}, "url"},
		{"channel url", GlobalFlags{}, []string{"read", "--url", "https://acme.slack.com/archives/C0123456789"}, "message"},
		{"raw with --save-images", GlobalFlags{Raw: true}, []string{"read", "--url", messageURL, "--save-images", "x"}, "--save-images"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := tc.global
			cmd := newThreadCommand(&g)
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.wantErr)
			}
		})
	}
}
