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

	// Sanity: section IDs for top-level elements (headers, paragraphs, list items)
	// must all appear in the sections map. IDs nested inside <table> cells are
	// intentionally omitted by the converter (collectCellText bypasses capture).
	// We check the double-quoted-attribute form only; single-quoted attrs (on <ul>
	// and <li> wrapper elements) may or may not be present depending on depth.
	inStr := string(in)
	tableStart := strings.Index(inStr, "<table>")
	tableEnd := strings.Index(inStr, "</table>") + len("</table>")
	parts := strings.Split(inStr, `id="temp:C:`)
	for i, line := range parts[1:] {
		end := strings.Index(line, `"`)
		if end <= 0 {
			continue
		}
		id := "temp:C:" + line[:end]
		// Locate this id's byte offset in the original input to skip table-nested ids.
		offset := strings.Index(inStr, `id="temp:C:`+line[:end]+`"`)
		if tableStart >= 0 && tableEnd > tableStart && offset >= tableStart && offset < tableEnd {
			continue // table-cell <p> ids are not captured by the converter
		}
		_ = i
		if _, ok := sections[id]; !ok {
			t.Errorf("missing section mapping for %s", id)
		}
	}
}
