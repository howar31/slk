package commands

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestMsgItem_ConciseShowsFiles(t *testing.T) {
	base := msgItem{User: "Alice", Text: "see this", TS: "1700000000.000100"}
	stamp := shortTS(base.TS)
	cases := []struct {
		name  string
		files []msgFile
		want  string
	}{
		{"no files", nil, "Alice: see this [" + stamp + "]"},
		{
			"listed file",
			[]msgFile{{ID: "F01234567", Name: "image.png", Mimetype: "image/png", Size: 10}},
			"Alice: see this [" + stamp + "] [file F01234567 image.png image/png]",
		},
		{
			"saved image",
			[]msgFile{{ID: "F01234567", Name: "image.png", Mimetype: "image/png", Path: "/tmp/a.png"}},
			"Alice: see this [" + stamp + "] [file F01234567 image.png image/png saved /tmp/a.png]",
		},
		{
			"failed image",
			[]msgFile{{ID: "F01234567", Name: "image.png", SaveError: "boom"}},
			"Alice: see this [" + stamp + "] [file F01234567 image.png save failed: boom]",
		},
		{
			"stub with only an ID, plus a second file",
			[]msgFile{{ID: "F0000000A"}, {ID: "F0000000B", Name: "b.png"}},
			"Alice: see this [" + stamp + "] [file F0000000A] [file F0000000B b.png]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := base
			item.Files = tc.files
			if got := item.Concise(); got != tc.want {
				t.Errorf("Concise() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMsgFiles_FromSlackMessage(t *testing.T) {
	raw := `{"text":"","files":[
		{"id":"F0000000A","name":"image.png","mimetype":"image/png","size":79174,"mode":"hosted",
		 "url_private":"https://files.slack.com/files-pri/T0-F0000000A/image.png"},
		{"id":"F0000000B","mode":"tombstone"}]}`
	var m slackMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}

	got := msgFiles(m.Files)

	want := []msgFile{
		{ID: "F0000000A", Name: "image.png", Mimetype: "image/png", Size: 79174},
		{ID: "F0000000B"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("msgFiles = %+v, want %+v", got, want)
	}
}

func TestMsgItem_JSONOmitsFilesWhenNone(t *testing.T) {
	b, err := json.Marshal(msgItem{User: "Alice", Text: "hi", TS: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "files") {
		t.Errorf("json = %s", b)
	}
}

func TestParseMessageURL(t *testing.T) {
	cases := []struct {
		name, url string
		want      messageRef
	}{
		{"channel", "https://acme.slack.com/archives/C0123456789", messageRef{Channel: "C0123456789"}},
		{"channel with trailing slash", "https://acme.slack.com/archives/C0123456789/", messageRef{Channel: "C0123456789"}},
		{"message", "https://acme.slack.com/archives/C0123456789/p1700000000123456",
			messageRef{Channel: "C0123456789", TS: "1700000000.123456"}},
		{"dm message", "https://acme.slack.com/archives/D0123456789/p1700000000123456",
			messageRef{Channel: "D0123456789", TS: "1700000000.123456"}},
		{"reply", "https://acme.slack.com/archives/C0123456789/p1700000000123456?thread_ts=1700000000.000100&cid=C0123456789",
			messageRef{Channel: "C0123456789", TS: "1700000000.123456", ThreadTS: "1700000000.000100"}},
		{"enterprise host", "https://acme.enterprise.slack.com/archives/C0123456789/p1700000000123456",
			messageRef{Channel: "C0123456789", TS: "1700000000.123456"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMessageURL(tc.url)
			if err != nil {
				t.Fatalf("parseMessageURL: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseMessageURL_Rejects(t *testing.T) {
	cases := []string{
		"",
		"C0123456789",
		"https://app.slack.com/client/T0123456789/C0123456789",
		"https://acme.slack.com/archives/",
		"https://acme.slack.com/archives/C0123456789/1700000000123456",
		"https://acme.slack.com/archives/C0123456789/p123",
		"https://acme.slack.com/archives/C0123456789/p17000000001234xy",
		"https://acme.slack.com/archives/C0123456789/p1700000000123456/extra",
	}
	for _, url := range cases {
		if got, err := parseMessageURL(url); err == nil {
			t.Errorf("parseMessageURL(%q) = %+v, want an error", url, got)
		}
	}
}

// fakeMessageAPI serves one canned response and records the call it received.
func fakeMessageAPI(t *testing.T, response string) (*api.Client, *string, *map[string]string) {
	t.Helper()
	var path string
	params := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		path = r.URL.Path
		for k := range r.Form {
			params[k] = r.Form.Get(k)
		}
		fmt.Fprint(w, response)
	}))
	t.Cleanup(srv.Close)
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL
	return c, &path, &params
}

func TestFetchMessageByRef_TopLevelMessage(t *testing.T) {
	c, path, params := fakeMessageAPI(t, `{"ok":true,"messages":[{"text":"hello","ts":"1700000000.123456"}]}`)

	_, msgs, err := fetchMessageByRef(c, messageRef{Channel: "C0123456789", TS: "1700000000.123456"})
	if err != nil {
		t.Fatalf("fetchMessageByRef: %v", err)
	}

	if *path != "/conversations.history" {
		t.Errorf("path = %s", *path)
	}
	for k, want := range map[string]string{
		"channel": "C0123456789", "oldest": "1700000000.123456", "latest": "1700000000.123456", "inclusive": "true",
	} {
		if (*params)[k] != want {
			t.Errorf("param %s = %q, want %q", k, (*params)[k], want)
		}
	}
	if len(msgs) != 1 || msgs[0].Text != "hello" {
		t.Errorf("msgs = %+v", msgs)
	}
}

func TestFetchMessageByRef_ReplyDropsThreadParent(t *testing.T) {
	c, path, params := fakeMessageAPI(t, `{"ok":true,"messages":[
		{"text":"parent","ts":"1700000000.000100","thread_ts":"1700000000.000100"},
		{"text":"reply","ts":"1700000000.123456","thread_ts":"1700000000.000100"}]}`)

	_, msgs, err := fetchMessageByRef(c, messageRef{
		Channel: "C0123456789", TS: "1700000000.123456", ThreadTS: "1700000000.000100",
	})
	if err != nil {
		t.Fatalf("fetchMessageByRef: %v", err)
	}

	if *path != "/conversations.replies" || (*params)["ts"] != "1700000000.000100" {
		t.Errorf("path = %s, ts = %q", *path, (*params)["ts"])
	}
	if len(msgs) != 1 || msgs[0].Text != "reply" {
		t.Errorf("msgs = %+v", msgs)
	}
}

func TestFetchMessageByRef_ThreadParentLinkUsesHistory(t *testing.T) {
	c, path, _ := fakeMessageAPI(t, `{"ok":true,"messages":[{"text":"parent","ts":"1700000000.000100"}]}`)

	_, _, err := fetchMessageByRef(c, messageRef{
		Channel: "C0123456789", TS: "1700000000.000100", ThreadTS: "1700000000.000100",
	})
	if err != nil {
		t.Fatalf("fetchMessageByRef: %v", err)
	}
	if *path != "/conversations.history" {
		t.Errorf("path = %s", *path)
	}
}

func TestFetchMessageByRef_MissingMessageExits4(t *testing.T) {
	c, _, _ := fakeMessageAPI(t, `{"ok":true,"messages":[]}`)

	_, _, err := fetchMessageByRef(c, messageRef{Channel: "C0123456789", TS: "1700000000.123456"})

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.ExitCode() != 4 {
		t.Fatalf("err = %v, want an APIError with exit code 4", err)
	}
}

func TestMsgRead_FlagValidation(t *testing.T) {
	const messageURL = "https://acme.slack.com/archives/C0123456789/p1700000000123456"
	cases := []struct {
		name    string
		global  GlobalFlags
		args    []string
		wantErr string
	}{
		{"neither channel nor url", GlobalFlags{}, []string{"read"}, "channel"},
		{"channel and url", GlobalFlags{}, []string{"read", "--channel", "C0123456789", "--url", messageURL}, "url"},
		{"bad url", GlobalFlags{}, []string{"read", "--url", "https://acme.slack.com/x"}, "archives"},
		{"message url with --oldest", GlobalFlags{}, []string{"read", "--url", messageURL, "--oldest", "1.0"}, "--oldest"},
		{"raw with --save-images", GlobalFlags{Raw: true}, []string{"read", "--channel", "C0123456789", "--save-images", "x"}, "--save-images"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := tc.global
			cmd := newMsgCommand(&g)
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
