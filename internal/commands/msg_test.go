package commands

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestMsgSchedule_ThreadAndBroadcastFlags(t *testing.T) {
	g := &GlobalFlags{}
	cmd := newMsgCommand(g)
	sch, _, err := cmd.Find([]string{"schedule"})
	if err != nil {
		t.Fatalf("find schedule: %v", err)
	}
	for _, name := range []string{"thread", "reply-broadcast"} {
		if sch.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s on msg schedule", name)
		}
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

func TestMsgDraft_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newMsgCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"draft", "--channel", "C1", "--text", "hello draft"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "drafts.create") || !strings.Contains(out.String(), "hello draft") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}

func TestNewUUID_FormatV4(t *testing.T) {
	u := newUUID()
	parts := strings.Split(u, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 segments, got %d (%q)", len(parts), u)
	}
	if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Fatalf("segment lengths wrong: %v", parts)
	}
	if parts[2][0] != '4' {
		t.Errorf("version byte not 4: %s", parts[2])
	}
}

func TestTextToBlocks_RoundTrip(t *testing.T) {
	out := textToBlocks("hi")
	if !strings.Contains(out, `"rich_text"`) || !strings.Contains(out, `"text":"hi"`) {
		t.Fatalf("blocks JSON malformed: %s", out)
	}
}

func TestMsgSend_TextFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "m.txt")
	if err := os.WriteFile(p, []byte("from\nfile"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &GlobalFlags{DryRun: true}
	cmd := newMsgCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"send", "--channel", "C1", "--text-file", p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "from\\nfile") && !strings.Contains(out.String(), "from\nfile") {
		t.Fatalf("file content missing: %q", out.String())
	}
}

func TestMsgSend_RequiresTextOrFile(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newMsgCommand(g)
	cmd.SetArgs([]string{"send", "--channel", "C1"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when neither --text nor --text-file given")
	}
}
