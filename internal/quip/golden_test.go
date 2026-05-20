package quip

import (
	"os"
	"strings"
	"testing"
)

func TestConvert_FullFixture_Golden(t *testing.T) {
	in, err := os.ReadFile("testdata/canvas_fixture.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	want, err := os.ReadFile("testdata/canvas_fixture.md")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	got, sections, err := Convert(string(in))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if got != string(want) {
		t.Errorf("converted output diverged from golden.\n--- want ---\n%s\n--- got ---\n%s", string(want), got)
	}

	// Sanity: every temp:C: section ID found in the fixture HTML (double-quoted
	// attribute form) must appear in the sections map returned by Convert.
	// Single-quoted attrs (on <ul> and <li> wrapper elements) are skipped because
	// they use a different quoting style and are handled separately.
	inStr := string(in)
	parts := strings.Split(inStr, `id="temp:C:`)
	for _, line := range parts[1:] {
		end := strings.Index(line, `"`)
		if end <= 0 {
			continue
		}
		id := "temp:C:" + line[:end]
		if _, ok := sections[id]; !ok {
			t.Errorf("missing section mapping for %s", id)
		}
	}
}
