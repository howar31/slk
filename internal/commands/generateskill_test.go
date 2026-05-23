package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSkills_WritesTreeAndCleansStale(t *testing.T) {
	dir := t.TempDir()

	// A stale generated dir that must be removed on regeneration.
	staleDir := filepath.Join(dir, "slk-zzz")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand("9.9.9")
	root.SetArgs([]string{"generate-skills", "--output-dir", dir})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"slk/SKILL.md", "slk-shared/SKILL.md", "slk-msg/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("expected generated file %q: %v", want, err)
		}
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Errorf("stale dir %q should have been removed", staleDir)
	}
}

func TestCleanGeneratedSkills_OnlyRemovesSkillDirs(t *testing.T) {
	dir := t.TempDir()

	// A file named like the slk binary (matches the `slk` glob) — must survive,
	// so `generate-skills --output-dir .` cannot delete a built binary.
	binary := filepath.Join(dir, "slk")
	if err := os.WriteFile(binary, []byte("ELF"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory matching `slk-*` but without a SKILL.md — not generated, must survive.
	unrelated := filepath.Join(dir, "slk-data")
	if err := os.MkdirAll(unrelated, 0o755); err != nil {
		t.Fatal(err)
	}
	// A generated skill dir (has SKILL.md) — must be removed.
	generated := filepath.Join(dir, "slk-msg")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cleanGeneratedSkills(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(binary); err != nil {
		t.Error("a non-directory match (binary) must survive cleanup")
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Error("a matching dir without SKILL.md must survive cleanup")
	}
	if _, err := os.Stat(generated); !os.IsNotExist(err) {
		t.Error("a generated skill dir (with SKILL.md) must be removed")
	}
}

func TestGenerateSkills_Idempotent(t *testing.T) {
	dir := t.TempDir()
	gen := func() {
		root := NewRootCommand("9.9.9")
		root.SetArgs([]string{"generate-skills", "--output-dir", dir})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	gen()
	first, err := os.ReadFile(filepath.Join(dir, "slk", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	gen()
	second, err := os.ReadFile(filepath.Join(dir, "slk", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("generate-skills is not idempotent for slk/SKILL.md")
	}
}
