package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadContent_Inline(t *testing.T) {
	got, err := readContent("hello", "", "--text", "--text-file")
	if err != nil || got != "hello" {
		t.Fatalf("inline: got=%q err=%v", got, err)
	}
}

func TestReadContent_File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.md")
	if err := os.WriteFile(p, []byte("multi\nline"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readContent("", p, "--markdown", "--markdown-file")
	if err != nil || got != "multi\nline" {
		t.Fatalf("file: got=%q err=%v", got, err)
	}
}

func TestReadContent_NeitherIsError(t *testing.T) {
	if _, err := readContent("", "", "--text", "--text-file"); err == nil {
		t.Fatal("expected error when neither provided")
	}
}

func TestReadContent_BothIsError(t *testing.T) {
	if _, err := readContent("x", "y", "--text", "--text-file"); err == nil {
		t.Fatal("expected error when both provided")
	}
}

func TestReadContent_FileMissing(t *testing.T) {
	if _, err := readContent("", "/nonexistent/path/xxx", "--text", "--text-file"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadContent_FileStripWithStrings(t *testing.T) {
	// just a sanity check the helper preserves bytes verbatim
	dir := t.TempDir()
	p := filepath.Join(dir, "y.md")
	body := "line1\nline2\n\nline4"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readContent("", p, "--markdown", "--markdown-file")
	if err != nil {
		t.Fatal(err)
	}
	if got != body {
		t.Fatalf("body roundtrip: got %q want %q", got, body)
	}
	if !strings.Contains(got, "\n\n") {
		t.Fatal("blank line lost")
	}
}
