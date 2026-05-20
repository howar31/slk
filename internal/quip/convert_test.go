package quip

import (
	"strings"
	"testing"
)

func TestConvert_Headers(t *testing.T) {
	in := `<h1 id="temp:C:a1">Title</h1><h2 id="temp:C:a2">Section</h2>`
	got, sec, err := Convert(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "# Title") {
		t.Errorf("missing h1: %q", got)
	}
	if !strings.Contains(got, "## Section") {
		t.Errorf("missing h2: %q", got)
	}
	if sec["temp:C:a1"] == "" {
		t.Errorf("missing section map for a1: %v", sec)
	}
	if sec["temp:C:a2"] == "" {
		t.Errorf("missing section map for a2: %v", sec)
	}
}

func TestConvert_Paragraph(t *testing.T) {
	got, _, _ := Convert(`<p>hello <b>world</b></p>`)
	if !strings.Contains(got, "hello **world**") {
		t.Errorf("got %q", got)
	}
}

func TestConvert_EmptyParagraph(t *testing.T) {
	got, _, _ := Convert(`<p></p><p>real</p>`)
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("empty paragraph emitted extra blank lines: %q", got)
	}
	if !strings.Contains(got, "real") {
		t.Errorf("real paragraph missing: %q", got)
	}
}

func TestConvert_QuipLink(t *testing.T) {
	got, _, _ := Convert(`<p>See <lnk href="https://x.test">here</lnk>.</p>`)
	if !strings.Contains(got, "[here](https://x.test)") {
		t.Errorf("got %q", got)
	}
}

func TestConvert_StandardLink(t *testing.T) {
	got, _, _ := Convert(`<p><a href="https://example.com">link</a></p>`)
	if !strings.Contains(got, "[link](https://example.com)") {
		t.Errorf("got %q", got)
	}
}

func TestConvert_CodeBlock(t *testing.T) {
	got, _, _ := Convert(`<p class="prettyprint">func main() {<br>    fmt.Println("hi")<br>}</p>`)
	if !strings.Contains(got, "```") {
		t.Errorf("missing fence: %q", got)
	}
	if !strings.Contains(got, `fmt.Println("hi")`) {
		t.Errorf("missing body: %q", got)
	}
	if !strings.Contains(got, "func main()") {
		t.Errorf("missing func main: %q", got)
	}
}

func TestConvert_InlineCode(t *testing.T) {
	got, _, _ := Convert(`<p>use <code>fmt.Println</code> here</p>`)
	if !strings.Contains(got, "`fmt.Println`") {
		t.Errorf("got %q", got)
	}
}

func TestConvert_Checklist(t *testing.T) {
	in := `<div data-section-style="7"><ul><li><span>todo</span></li><li class="checked"><span>done</span></li></ul></div>`
	got, _, _ := Convert(in)
	if !strings.Contains(got, "* [ ] todo") || !strings.Contains(got, "* [x] done") {
		t.Errorf("checklist failed: %q", got)
	}
}

func TestConvert_OrderedList(t *testing.T) {
	in := `<div data-section-style="6"><ul><li value="1"><span>First</span></li><li value="2"><span>Second</span></li></ul></div>`
	got, _, _ := Convert(in)
	if !strings.Contains(got, "1. First") || !strings.Contains(got, "2. Second") {
		t.Errorf("ordered list failed: %q", got)
	}
}

func TestConvert_BulletList(t *testing.T) {
	in := `<ul><li><span>A</span></li><li><span>B</span></li></ul>`
	got, _, _ := Convert(in)
	if !strings.Contains(got, "* A") || !strings.Contains(got, "* B") {
		t.Errorf("bullet list failed: %q", got)
	}
}

func TestConvert_NestedList(t *testing.T) {
	in := `<ul><li><span>A</span></li><li><span>B</span></li><ul><li><span>B1</span></li></ul><li><span>C</span></li></ul>`
	got, _, _ := Convert(in)
	if !strings.Contains(got, "* A") {
		t.Errorf("missing A in: %q", got)
	}
	if !strings.Contains(got, "* B") {
		t.Errorf("missing B in: %q", got)
	}
	if !strings.Contains(got, "    * B1") {
		t.Errorf("missing indented B1 in: %q", got)
	}
	if !strings.Contains(got, "* C") {
		t.Errorf("missing C in: %q", got)
	}
}

func TestConvert_Blockquote(t *testing.T) {
	got, _, _ := Convert(`<blockquote><p>quoted line</p></blockquote>`)
	if !strings.Contains(got, "> quoted line") {
		t.Errorf("got %q", got)
	}
}

func TestConvert_Table(t *testing.T) {
	in := `<table><tr><td>A</td><td>B</td></tr><tr><td>1</td><td>2</td></tr></table>`
	got, _, _ := Convert(in)
	if !strings.Contains(got, "| A | B |") {
		t.Errorf("missing header row: %q", got)
	}
	if !strings.Contains(got, "| --- | --- |") {
		t.Errorf("missing separator row: %q", got)
	}
	if !strings.Contains(got, "| 1 | 2 |") {
		t.Errorf("missing data row: %q", got)
	}
}

func TestConvert_TableWithTh(t *testing.T) {
	in := `<table><tr><th>Col1</th><th>Col2</th></tr><tr><td>val1</td><td>val2</td></tr></table>`
	got, _, _ := Convert(in)
	if !strings.Contains(got, "| Col1 | Col2 |") {
		t.Errorf("missing th header: %q", got)
	}
	if !strings.Contains(got, "| --- | --- |") {
		t.Errorf("missing separator: %q", got)
	}
}

func TestConvert_StrikeBoldItalicCode(t *testing.T) {
	got, _, _ := Convert(`<p><b>b</b> <i>i</i> <del>s</del> <code>c</code></p>`)
	for _, want := range []string{"**b**", "*i*", "~~s~~", "`c`"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestConvert_StrongAndEm(t *testing.T) {
	got, _, _ := Convert(`<p><strong>bold</strong> <em>italic</em></p>`)
	if !strings.Contains(got, "**bold**") {
		t.Errorf("missing strong->bold: %q", got)
	}
	if !strings.Contains(got, "*italic*") {
		t.Errorf("missing em->italic: %q", got)
	}
}

func TestConvert_STag(t *testing.T) {
	got, _, _ := Convert(`<p><s>struck</s></p>`)
	if !strings.Contains(got, "~~struck~~") {
		t.Errorf("missing <s> strikethrough: %q", got)
	}
}

func TestConvert_SectionIDs(t *testing.T) {
	in := `<h1 id="temp:C:abc">Hello</h1><p id="temp:C:def">World</p>`
	_, sec, err := Convert(in)
	if err != nil {
		t.Fatal(err)
	}
	if sec["temp:C:abc"] == "" {
		t.Errorf("section abc missing from map: %v", sec)
	}
	if sec["temp:C:def"] == "" {
		t.Errorf("section def missing from map: %v", sec)
	}
	if !strings.Contains(sec["temp:C:abc"], "Hello") {
		t.Errorf("section abc content wrong: %q", sec["temp:C:abc"])
	}
}

func TestConvert_UnknownTagFallthrough(t *testing.T) {
	// Unknown tags should just emit their text content.
	got, _, _ := Convert(`<article><p>text</p></article>`)
	if !strings.Contains(got, "text") {
		t.Errorf("unknown tag dropped text: %q", got)
	}
}

func TestConvert_BrNewline(t *testing.T) {
	got, _, _ := Convert(`<p>line1<br/>line2</p>`)
	if !strings.Contains(got, "line1\nline2") {
		t.Errorf("br not converted to newline: %q", got)
	}
}

func TestConvert_NoLeadingBlankLine(t *testing.T) {
	got, _, _ := Convert(`<h1 id="x">T</h1>`)
	if strings.HasPrefix(got, "\n") {
		t.Fatalf("output starts with newline: %q", got)
	}
	if !strings.HasPrefix(got, "# T") {
		t.Fatalf("expected to start with '# T', got %q", got)
	}
}

func TestConvert_BlockquoteSkipsEmptyParagraph(t *testing.T) {
	in := `<p id="a"></p><blockquote><p id="b">quoted</p></blockquote>`
	got, _, _ := Convert(in)
	if strings.Contains(got, "> \n") || strings.Contains(got, "> \n>") {
		t.Fatalf("empty blockquote line leaked: %q", got)
	}
	if !strings.Contains(got, "> quoted") {
		t.Fatalf("missing real blockquote: %q", got)
	}
}

// TestConvert_Integration tests a comprehensive canvas HTML with multiple element families.
func TestConvert_Integration(t *testing.T) {
	const fixture = `
<h1 id="temp:C:h1">Project Plan</h1>
<p id="temp:C:p1">This is a <b>bold</b> and <i>italic</i> paragraph with <code>inline code</code>.</p>
<h2>Tasks</h2>
<div data-section-style="7">
  <ul>
    <li><span>Write tests</span></li>
    <li class="checked"><span>Set up repo</span></li>
  </ul>
</div>
<h2>Steps</h2>
<div data-section-style="6">
  <ul>
    <li value="1"><span>Plan</span></li>
    <li value="2"><span>Implement</span></li>
    <li value="3"><span>Review</span></li>
  </ul>
</div>
<h2>Notes</h2>
<ul>
  <li><span>Top level A</span></li>
  <li><span>Top level B</span></li>
  <ul>
    <li><span>Nested B1</span></li>
    <li><span>Nested B2</span></li>
  </ul>
  <li><span>Top level C</span></li>
</ul>
<blockquote><p>An important note.</p></blockquote>
<p class="prettyprint">package main<br>func main() {<br>    println("hello")<br>}</p>
<p>See <lnk href="https://slack.com">Slack</lnk> and <a href="https://github.com">GitHub</a>.</p>
<table>
  <tr><th>Name</th><th>Status</th></tr>
  <tr><td>Alpha</td><td>Done</td></tr>
  <tr><td>Beta</td><td>In Progress</td></tr>
</table>
`
	got, sec, err := Convert(fixture)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}

	checks := []struct {
		desc string
		want string
	}{
		{"h1", "# Project Plan"},
		{"h2", "## Tasks"},
		{"bold", "**bold**"},
		{"italic", "*italic*"},
		{"inline code", "`inline code`"},
		{"checklist unchecked", "* [ ] Write tests"},
		{"checklist checked", "* [x] Set up repo"},
		{"ordered 1", "1. Plan"},
		{"ordered 2", "2. Implement"},
		{"ordered 3", "3. Review"},
		{"bullet A", "* Top level A"},
		{"bullet B", "* Top level B"},
		{"nested B1", "    * Nested B1"},
		{"nested B2", "    * Nested B2"},
		{"bullet C", "* Top level C"},
		{"blockquote", "> An important note."},
		{"code fence", "```"},
		{"code body", `println("hello")`},
		{"quip link", "[Slack](https://slack.com)"},
		{"standard link", "[GitHub](https://github.com)"},
		{"table header", "| Name | Status |"},
		{"table separator", "| --- | --- |"},
		{"table row", "| Alpha | Done |"},
	}

	for _, tc := range checks {
		if !strings.Contains(got, tc.want) {
			t.Errorf("integration: missing %s (%q) in output:\n%s", tc.desc, tc.want, got)
		}
	}

	// Section IDs should be captured.
	if sec["temp:C:h1"] == "" {
		t.Errorf("section temp:C:h1 not captured")
	}
	if sec["temp:C:p1"] == "" {
		t.Errorf("section temp:C:p1 not captured")
	}
}
