package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestMsgMessage_Concise(t *testing.T) {
	m := msgItem{User: "Bob", Text: "hi", TS: "1779191572.0"}
	got := m.Concise()
	if !strings.Contains(got, "Bob") || !strings.Contains(got, "hi") {
		t.Fatalf("concise = %q: missing User or Text", got)
	}
	if !strings.Contains(got, "[") {
		t.Fatalf("concise = %q: missing timestamp bracket (shortTS output)", got)
	}
}

func TestMsgSend_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newMsgCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"send", "--channel", "C1", "--text", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "hello") || !strings.Contains(out.String(), "C1") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}

func TestMsgRead_FlagsRegistered(t *testing.T) {
	g := &GlobalFlags{}
	cmd := newMsgCommand(g)
	read, _, err := cmd.Find([]string{"read"})
	if err != nil {
		t.Fatalf("find read: %v", err)
	}
	for _, name := range []string{"oldest", "latest", "cursor", "channel", "limit"} {
		if read.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s on msg read", name)
		}
	}
}

func TestMsgSend_ReplyBroadcastFlag(t *testing.T) {
	g := &GlobalFlags{}
	cmd := newMsgCommand(g)
	send, _, err := cmd.Find([]string{"send"})
	if err != nil {
		t.Fatalf("find send: %v", err)
	}
	if send.Flags().Lookup("reply-broadcast") == nil {
		t.Fatal("missing --reply-broadcast flag")
	}
}

func TestMsgUpdateDelete_DryRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"update", []string{"update", "--channel", "C1", "--ts", "1.0", "--text", "new"}, "chat.update"},
		{"delete", []string{"delete", "--channel", "C1", "--ts", "1.0"}, "chat.delete"},
		{"react", []string{"react", "--channel", "C1", "--ts", "1.0", "--emoji", "thumbsup"}, "reactions.add"},
		{"schedule", []string{"schedule", "--channel", "C1", "--text", "later", "--at", "1799999999"}, "chat.scheduleMessage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &GlobalFlags{DryRun: true}
			cmd := newMsgCommand(g)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("want %q in %q", tc.want, out.String())
			}
		})
	}
}
