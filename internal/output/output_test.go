package output

import (
	"bytes"
	"strings"
	"testing"
)

type sampleMsg struct {
	User string `json:"user"`
	Text string `json:"text"`
}

func (m sampleMsg) Concise() string { return m.User + ": " + m.Text }

func TestEmit_Concise(t *testing.T) {
	var buf bytes.Buffer
	items := []sampleMsg{{"Bob", "hi"}, {"Alice", "yo"}}
	if err := Emit(&buf, "concise", items); err != nil {
		t.Fatalf("emit: %v", err)
	}
	want := "Bob: hi\nAlice: yo\n"
	if buf.String() != want {
		t.Fatalf("concise = %q, want %q", buf.String(), want)
	}
}

func TestEmit_JSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Emit(&buf, "json", []sampleMsg{{"Bob", "hi"}}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if !strings.Contains(buf.String(), `"user": "Bob"`) {
		t.Fatalf("json missing field: %s", buf.String())
	}
}

func TestEmit_JSONL(t *testing.T) {
	var buf bytes.Buffer
	if err := Emit(&buf, "jsonl", []sampleMsg{{"Bob", "hi"}, {"Alice", "yo"}}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	lines := strings.Count(strings.TrimSpace(buf.String()), "\n") + 1
	if lines != 2 {
		t.Fatalf("jsonl expected 2 lines, got %d", lines)
	}
}

func TestEmit_Table(t *testing.T) {
	var buf bytes.Buffer
	if err := Emit(&buf, "table", []sampleMsg{{"Bob", "hi"}}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "USER") || !strings.Contains(out, "Bob") {
		t.Fatalf("table missing header/row: %s", out)
	}
}

func TestEmit_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Emit(&buf, "xml", []sampleMsg{{"Bob", "hi"}}); err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestEmit_TableEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Emit(&buf, "table", []sampleMsg{}); err != nil {
		t.Fatalf("emit empty table: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("empty table should produce no output, got %q", buf.String())
	}
}

func TestEmit_NonSlice(t *testing.T) {
	var buf bytes.Buffer
	if err := Emit(&buf, "json", sampleMsg{"Bob", "hi"}); err == nil {
		t.Fatal("expected error when items is not a slice")
	}
}
