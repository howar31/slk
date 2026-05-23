package commands

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestPromptLine_TrimsAndWritesPrompt(t *testing.T) {
	in := strings.NewReader("  hello \n")
	var w bytes.Buffer
	got, err := promptLine(in, &w, "Name: ")
	if err != nil {
		t.Fatalf("promptLine: %v", err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
	if !strings.Contains(w.String(), "Name: ") {
		t.Fatalf("prompt not written: %q", w.String())
	}
}

func TestReadLine_NoTrailingNewline(t *testing.T) {
	got, err := readLine(strings.NewReader("xoxp-piped"))
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if got != "xoxp-piped" {
		t.Fatalf("got %q", got)
	}
}

func TestReadLine_StopsAtNewlineWithoutOverReading(t *testing.T) {
	r := strings.NewReader("first\nsecond\n")
	got, err := readLine(r)
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if got != "first" {
		t.Fatalf("got %q, want %q", got, "first")
	}
	rest, _ := io.ReadAll(r)
	if string(rest) != "second\n" {
		t.Fatalf("readLine over-read; remaining = %q, want %q", rest, "second\n")
	}
}

func TestPromptSecret_UsesSeamAndTrims(t *testing.T) {
	orig := readSecret
	t.Cleanup(func() { readSecret = orig })
	readSecret = func(int) ([]byte, error) { return []byte(" xoxp-secret \n"), nil }

	var w bytes.Buffer
	got, err := promptSecret(0, &w, "Token: ")
	if err != nil {
		t.Fatalf("promptSecret: %v", err)
	}
	if got != "xoxp-secret" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(w.String(), "Token: ") {
		t.Fatalf("prompt not written: %q", w.String())
	}
}

func TestResolveLine_FlagWins(t *testing.T) {
	got, err := resolveLine(nil, io.Discard, "work", true, true, "Profile: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "work" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveLine_NonInteractiveEmpty(t *testing.T) {
	got, err := resolveLine(nil, io.Discard, "", false, false, "Profile: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveLine_InteractivePrompts(t *testing.T) {
	got, err := resolveLine(strings.NewReader("acme\n"), io.Discard, "", false, true, "Workspace: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "acme" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_StdinDash(t *testing.T) {
	got, err := resolveSecret(strings.NewReader("xoxp-piped\n"), io.Discard, 0, "-", true, false, false, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "xoxp-piped" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_FlagValueWins(t *testing.T) {
	got, err := resolveSecret(nil, io.Discard, 0, "xoxp-flag", true, true, true, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "xoxp-flag" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_NonInteractiveEmpty(t *testing.T) {
	got, err := resolveSecret(nil, io.Discard, 0, "", false, false, true, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_RequiredRepromptsUntilNonEmpty(t *testing.T) {
	orig := readSecret
	t.Cleanup(func() { readSecret = orig })
	vals := []string{"", "", "xoxp-final"}
	readSecret = func(int) ([]byte, error) {
		v := vals[0]
		vals = vals[1:]
		return []byte(v), nil
	}
	got, err := resolveSecret(nil, io.Discard, 0, "", false, true, true, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "xoxp-final" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_OptionalAcceptsEmpty(t *testing.T) {
	orig := readSecret
	t.Cleanup(func() { readSecret = orig })
	readSecret = func(int) ([]byte, error) { return []byte(""), nil }
	got, err := resolveSecret(nil, io.Discard, 0, "", false, true, false, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q", got)
	}
}
