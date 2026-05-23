package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howar31/slk/internal/api"
)

func TestParseScheduledMessageID(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"present", `{"ok":true,"scheduled_message_id":"Q0B5","channel":"C0","post_at":1779200000}`, "Q0B5"},
		{"missing", `{"ok":true}`, ""},
		{"malformed", `not json`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseScheduledMessageID([]byte(tc.raw)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMessageDisplay_Fallbacks(t *testing.T) {
	mk := func(user, username, botName, botID string) slackMessage {
		m := slackMessage{User: user, Username: username, BotID: botID}
		m.BotProfile.Name = botName
		return m
	}
	cases := []struct {
		name string
		m    slackMessage
		want string
	}{
		{"user wins over username", mk("U1", "ignored", "ignored", "B1"), "U1"},
		{"username when user empty", mk("", "incoming-webhook", "ignored", "B1"), "incoming-webhook"},
		{"bot profile name when username empty", mk("", "", "kintai", "B1"), "kintai"},
		{"bot id when nothing else", mk("", "", "", "B1"), "B1"},
		{"empty everything", mk("", "", "", ""), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := messageDisplay(nil, tc.m); got != tc.want {
				t.Fatalf("messageDisplay = %q, want %q", got, tc.want)
			}
		})
	}
}

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

// --- flag-registration tests for the 8 new verbs ---

func TestMsgNewVerbs_FlagsRegistered(t *testing.T) {
	cases := []struct {
		verb  string
		flags []string
	}{
		{"unreact", []string{"channel", "ts", "emoji"}},
		{"unschedule", []string{"channel", "id"}},
		{"scheduled", []string{"channel"}},
		{"permalink", []string{"channel", "ts"}},
		{"ephemeral", []string{"channel", "user", "text", "text-file"}},
		{"me", []string{"channel", "text"}},
		{"reactions", []string{"channel", "ts"}},
		{"reacted", []string{"user"}},
	}
	for _, tc := range cases {
		t.Run(tc.verb, func(t *testing.T) {
			g := &GlobalFlags{}
			cmd := newMsgCommand(g)
			sub, _, err := cmd.Find([]string{tc.verb})
			if err != nil {
				t.Fatalf("find %s: %v", tc.verb, err)
			}
			for _, name := range tc.flags {
				if sub.Flags().Lookup(name) == nil {
					t.Errorf("missing flag --%s on msg %s", name, tc.verb)
				}
			}
		})
	}
}

// --- dry-run table for the new WRITE verbs ---

func TestMsgNewWriteVerbs_DryRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			"unreact",
			[]string{"unreact", "--channel", "C0123456789", "--ts", "1.0", "--emoji", "thumbsup"},
			"reactions.remove",
		},
		{
			"unschedule",
			[]string{"unschedule", "--channel", "C0123456789", "--id", "Q0123"},
			"chat.deleteScheduledMessage",
		},
		{
			"ephemeral",
			[]string{"ephemeral", "--channel", "C0123456789", "--user", "U0123456789", "--text", "hi"},
			"chat.postEphemeral",
		},
		{
			"me",
			[]string{"me", "--channel", "C0123456789", "--text", "waves"},
			"chat.meMessage",
		},
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

// --- httptest-based parse-helper tests for the 3 new READ verbs ---

func TestParseMsgScheduled(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"scheduled_messages": [
			{"id": "Q0123456789", "channel_id": "C0123456789", "post_at": 1799999999, "text": "hello Alice"}
		]
	}`)
	hits, err := parseMsgScheduled(raw)
	if err != nil {
		t.Fatalf("parseMsgScheduled: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].ID != "Q0123456789" {
		t.Errorf("ID = %q, want Q0123456789", hits[0].ID)
	}
	if hits[0].Name != "hello Alice" {
		t.Errorf("Name = %q, want 'hello Alice'", hits[0].Name)
	}
	if hits[0].Extra != "1799999999" {
		t.Errorf("Extra = %q, want '1799999999'", hits[0].Extra)
	}
}

func TestMsgScheduled_Httptest(t *testing.T) {
	payload := map[string]interface{}{
		"ok": true,
		"scheduled_messages": []map[string]interface{}{
			{"id": "Q0123456789", "channel_id": "C0123456789", "post_at": 1799999999, "text": "hello Bob"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("chat.scheduledMessages.list", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	hits, err := parseMsgScheduled(raw)
	if err != nil {
		t.Fatalf("parseMsgScheduled: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "Q0123456789" {
		t.Fatalf("unexpected hits: %v", hits)
	}
}

func TestParseMsgReactions(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"type": "message",
		"message": {
			"reactions": [
				{"name": "thumbsup", "count": 3},
				{"name": "heart", "count": 1}
			]
		}
	}`)
	hits, err := parseMsgReactions(raw)
	if err != nil {
		t.Fatalf("parseMsgReactions: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].Name != "thumbsup" || hits[0].Extra != "3" {
		t.Errorf("hits[0] = %+v", hits[0])
	}
	if hits[1].Name != "heart" || hits[1].Extra != "1" {
		t.Errorf("hits[1] = %+v", hits[1])
	}
}

func TestMsgReactions_Httptest(t *testing.T) {
	payload := map[string]interface{}{
		"ok":   true,
		"type": "message",
		"message": map[string]interface{}{
			"reactions": []map[string]interface{}{
				{"name": "wave", "count": 2},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("reactions.get", map[string]string{"channel": "C0123456789", "timestamp": "1.0"}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	hits, err := parseMsgReactions(raw)
	if err != nil {
		t.Fatalf("parseMsgReactions: %v", err)
	}
	if len(hits) != 1 || hits[0].Name != "wave" || hits[0].Extra != "2" {
		t.Fatalf("unexpected hits: %v", hits)
	}
}

func TestParseMsgReacted(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"items": [
			{"type": "message", "message": {"ts": "1779191572.000100"}},
			{"type": "file", "file": {"id": "F01234567"}}
		]
	}`)
	hits, err := parseMsgReacted(raw)
	if err != nil {
		t.Fatalf("parseMsgReacted: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].ID != "1779191572.000100" || hits[0].Extra != "message" {
		t.Errorf("hits[0] = %+v", hits[0])
	}
	if hits[1].ID != "F01234567" || hits[1].Extra != "file" {
		t.Errorf("hits[1] = %+v", hits[1])
	}
}

func TestMsgReacted_Httptest(t *testing.T) {
	payload := map[string]interface{}{
		"ok": true,
		"items": []map[string]interface{}{
			{
				"type":    "message",
				"message": map[string]interface{}{"ts": "1779191572.000200"},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("reactions.list", map[string]string{}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	hits, err := parseMsgReacted(raw)
	if err != nil {
		t.Fatalf("parseMsgReacted: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "1779191572.000200" || hits[0].Extra != "message" {
		t.Fatalf("unexpected hits: %v", hits)
	}
}

func TestMsgPermalink_FlagsAndAnnotation(t *testing.T) {
	g := &GlobalFlags{}
	cmd := newMsgCommand(g)
	sub, _, err := cmd.Find([]string{"permalink"})
	if err != nil {
		t.Fatalf("find permalink: %v", err)
	}
	if sub.Annotations["slackMethod"] != "chat.getPermalink" {
		t.Errorf("annotation slackMethod = %q", sub.Annotations["slackMethod"])
	}
	for _, name := range []string{"channel", "ts"} {
		if sub.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s on msg permalink", name)
		}
	}
}

func TestMsgPermalink_Httptest(t *testing.T) {
	wantLink := fmt.Sprintf("https://example.slack.com/archives/C0123456789/p1779191572000100")
	payload := map[string]interface{}{
		"ok":        true,
		"channel":   "C0123456789",
		"permalink": wantLink,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("chat.getPermalink", map[string]string{"channel": "C0123456789", "message_ts": "1779191572.000100"}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp struct {
		Permalink string `json:"permalink"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if resp.Permalink != wantLink {
		t.Errorf("permalink = %q, want %q", resp.Permalink, wantLink)
	}
}
