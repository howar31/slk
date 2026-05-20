// Package quip converts Slack canvas (quip) HTML to Markdown.
package quip

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// Convert parses canvas HTML and returns the equivalent Canvas-flavored Markdown
// plus a mapping of section IDs (the `id="temp:C:..."` attributes Slack puts on
// every content element) to that element's rendered markdown.
func Convert(htmlStr string) (string, map[string]string, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return "", nil, fmt.Errorf("quip: parse html: %w", err)
	}
	c := &converter{sections: map[string]string{}}
	c.walk(doc, &listState{})
	md := strings.TrimRight(c.out.String(), "\n") + "\n"
	return md, c.sections, nil
}

// listState carries list context through recursive descent.
type listState struct {
	depth    int    // current nesting depth (0 = not in list)
	style    string // "bullet", "ordered", "checklist"
	counter  int    // for ordered lists
	prevWasList bool // true when the previous sibling element was an <li>
}

type converter struct {
	out      strings.Builder
	sections map[string]string
}

// walk descends an HTML node tree, emitting markdown into c.out.
// ls carries the list-nesting state.
func (c *converter) walk(n *html.Node, ls *listState) {
	switch n.Type {
	case html.DocumentNode:
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			c.walk(ch, ls)
		}
	case html.TextNode:
		c.out.WriteString(n.Data)
	case html.ElementNode:
		c.handleElement(n, ls)
	default:
		// DoctypeNode, CommentNode – ignore
	}
}

func (c *converter) handleElement(n *html.Node, ls *listState) {
	tag := n.Data
	id := attr(n, "id")

	// For elements with a section ID, capture their rendered markdown.
	if strings.HasPrefix(id, "temp:C:") {
		captured := c.capture(n, ls)
		c.sections[id] = strings.TrimSpace(captured)
		c.out.WriteString(captured)
		return
	}

	switch tag {
	case "html", "head", "body":
		c.walkChildren(n, ls)

	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(tag[1] - '0')
		c.out.WriteString("\n" + strings.Repeat("#", level) + " ")
		c.walkChildren(n, ls)
		c.out.WriteString("\n")

	case "p":
		if hasCSSClass(n, "prettyprint") {
			// Quip code block: <p class="prettyprint">line<br>line</p>
			c.out.WriteString("\n```\n")
			c.emitCodeBlock(n)
			c.out.WriteString("\n```\n")
		} else {
			text := strings.TrimSpace(c.collectText(n))
			if text == "" {
				return
			}
			c.out.WriteString("\n")
			// Render with inline markup instead of plain text.
			c.walkChildren(n, ls)
			c.out.WriteString("\n")
		}

	case "b", "strong":
		c.out.WriteString("**")
		c.walkChildren(n, ls)
		c.out.WriteString("**")

	case "i", "em":
		c.out.WriteString("*")
		c.walkChildren(n, ls)
		c.out.WriteString("*")

	case "del", "s":
		c.out.WriteString("~~")
		c.walkChildren(n, ls)
		c.out.WriteString("~~")

	case "code":
		c.out.WriteString("`")
		c.walkChildren(n, ls)
		c.out.WriteString("`")

	case "blockquote":
		// Render children into a sub-builder, then prefix each line with "> ".
		sub := c.sub(n, ls)
		for _, line := range strings.Split(strings.TrimRight(sub, "\n"), "\n") {
			c.out.WriteString("\n> " + strings.TrimLeft(line, "\n"))
		}
		c.out.WriteString("\n")

	case "a", "lnk":
		href := attr(n, "href")
		text := strings.TrimSpace(c.collectText(n))
		if href != "" && text != "" {
			c.out.WriteString("[" + text + "](" + href + ")")
		} else if text != "" {
			c.out.WriteString(text)
		} else {
			c.walkChildren(n, ls)
		}

	case "br":
		c.out.WriteString("\n")

	case "div":
		style := attr(n, "data-section-style")
		switch style {
		case "6":
			// Ordered list wrapper.
			c.emitList(n, "ordered", ls.depth)
		case "7":
			// Checklist wrapper.
			c.emitList(n, "checklist", ls.depth)
		default:
			c.walkChildren(n, ls)
		}

	case "ul":
		// Determine list style from ancestor div (passed in via ls) or default bullet.
		style := ls.style
		if style == "" {
			style = "bullet"
		}
		c.emitListItems(n, style, ls.depth)

	case "ol":
		c.emitList(n, "ordered", ls.depth)

	case "li":
		// Handled by emitListItems; fall through if encountered standalone.
		c.walkChildren(n, ls)

	case "table":
		c.emitTable(n, ls)

	case "tr", "td", "th", "thead", "tbody", "tfoot":
		// Handled inside emitTable.
		c.walkChildren(n, ls)

	case "span":
		c.walkChildren(n, ls)

	default:
		// Unknown tag: recurse into children, emit text content.
		c.walkChildren(n, ls)
	}
}

// walkChildren walks all direct children of n.
func (c *converter) walkChildren(n *html.Node, ls *listState) {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.walk(ch, ls)
	}
}

// capture renders a subtree into a string without writing to c.out.
func (c *converter) capture(n *html.Node, ls *listState) string {
	saved := c.out
	c.out = strings.Builder{}
	c.handleElementBody(n, ls)
	result := c.out.String()
	c.out = saved
	return result
}

// handleElementBody renders an element's content (not its section-id wrapper).
func (c *converter) handleElementBody(n *html.Node, ls *listState) {
	// Temporarily clear id to avoid infinite recursion, then call handleElement.
	// We do this by directly dispatching to the switch body.
	// The simplest approach: re-invoke handleElement with a node copy that has no id.
	// Instead, we just duplicate the logic via a flag — but that's complex.
	// Simplest correct approach: call handleElement; since id won't start with temp:C: (we removed it above conceptually),
	// but we can't mutate the node. So we route through an inner helper.
	c.handleElementInner(n, ls)
}

// handleElementInner is the same as handleElement but skips the section-id capture branch.
func (c *converter) handleElementInner(n *html.Node, ls *listState) {
	tag := n.Data
	switch tag {
	case "html", "head", "body":
		c.walkChildren(n, ls)
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(tag[1] - '0')
		c.out.WriteString("\n" + strings.Repeat("#", level) + " ")
		c.walkChildren(n, ls)
		c.out.WriteString("\n")
	case "p":
		if hasCSSClass(n, "prettyprint") {
			c.out.WriteString("\n```\n")
			c.emitCodeBlock(n)
			c.out.WriteString("\n```\n")
		} else {
			text := strings.TrimSpace(c.collectText(n))
			if text == "" {
				return
			}
			c.out.WriteString("\n")
			c.walkChildren(n, ls)
			c.out.WriteString("\n")
		}
	case "b", "strong":
		c.out.WriteString("**")
		c.walkChildren(n, ls)
		c.out.WriteString("**")
	case "i", "em":
		c.out.WriteString("*")
		c.walkChildren(n, ls)
		c.out.WriteString("*")
	case "del", "s":
		c.out.WriteString("~~")
		c.walkChildren(n, ls)
		c.out.WriteString("~~")
	case "code":
		c.out.WriteString("`")
		c.walkChildren(n, ls)
		c.out.WriteString("`")
	case "blockquote":
		sub := c.sub(n, ls)
		for _, line := range strings.Split(strings.TrimRight(sub, "\n"), "\n") {
			c.out.WriteString("\n> " + strings.TrimLeft(line, "\n"))
		}
		c.out.WriteString("\n")
	case "a", "lnk":
		href := attr(n, "href")
		text := strings.TrimSpace(c.collectText(n))
		if href != "" && text != "" {
			c.out.WriteString("[" + text + "](" + href + ")")
		} else if text != "" {
			c.out.WriteString(text)
		} else {
			c.walkChildren(n, ls)
		}
	case "br":
		c.out.WriteString("\n")
	case "div":
		style := attr(n, "data-section-style")
		switch style {
		case "6":
			c.emitList(n, "ordered", ls.depth)
		case "7":
			c.emitList(n, "checklist", ls.depth)
		default:
			c.walkChildren(n, ls)
		}
	case "ul":
		style := ls.style
		if style == "" {
			style = "bullet"
		}
		c.emitListItems(n, style, ls.depth)
	case "ol":
		c.emitList(n, "ordered", ls.depth)
	case "li":
		c.walkChildren(n, ls)
	case "table":
		c.emitTable(n, ls)
	case "tr", "td", "th", "thead", "tbody", "tfoot":
		c.walkChildren(n, ls)
	case "span":
		c.walkChildren(n, ls)
	default:
		c.walkChildren(n, ls)
	}
}

// sub renders a node's children into a separate string.
func (c *converter) sub(n *html.Node, ls *listState) string {
	saved := c.out
	c.out = strings.Builder{}
	c.walkChildren(n, ls)
	result := c.out.String()
	c.out = saved
	return result
}

// emitList finds the <ul>/<ol> inside a wrapper and renders it.
func (c *converter) emitList(wrapper *html.Node, style string, depth int) {
	for ch := wrapper.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && (ch.Data == "ul" || ch.Data == "ol") {
			c.emitListItems(ch, style, depth)
			return
		}
	}
	// No ul/ol child found — recurse generically.
	c.walkChildren(wrapper, &listState{style: style, depth: depth})
}

// emitListItems renders all <li> and nested <ul>/<ol> children of a list node.
// Nested <ul>/<ol> nodes are treated as siblings (quip style), not wrapped in <li>.
func (c *converter) emitListItems(listNode *html.Node, style string, depth int) {
	indent := strings.Repeat("    ", depth)
	orderedCounter := 0

	for ch := listNode.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type != html.ElementNode {
			continue
		}
		if ch.Data == "ul" || ch.Data == "ol" {
			// Sibling nested list (quip style): increase depth.
			nestedStyle := style
			if ch.Data == "ol" {
				nestedStyle = "ordered"
			}
			c.emitListItems(ch, nestedStyle, depth+1)
			continue
		}
		if ch.Data != "li" {
			continue
		}

		// Extract the item text (inline content).
		itemText := c.renderInline(ch)

		switch style {
		case "ordered":
			// Use the value attribute if present, else increment.
			v := attr(ch, "value")
			if v != "" {
				fmt.Sscanf(v, "%d", &orderedCounter)
			} else {
				orderedCounter++
			}
			c.out.WriteString("\n" + indent + fmt.Sprintf("%d. ", orderedCounter) + itemText)
		case "checklist":
			if hasCSSClass(ch, "checked") {
				c.out.WriteString("\n" + indent + "* [x] " + itemText)
			} else {
				c.out.WriteString("\n" + indent + "* [ ] " + itemText)
			}
		default: // bullet
			c.out.WriteString("\n" + indent + "* " + itemText)
		}

		// Look for nested lists as direct children of <li> (non-quip style).
		for sub := ch.FirstChild; sub != nil; sub = sub.NextSibling {
			if sub.Type == html.ElementNode && (sub.Data == "ul" || sub.Data == "ol") {
				nestedStyle := style
				if sub.Data == "ol" {
					nestedStyle = "ordered"
				}
				c.emitListItems(sub, nestedStyle, depth+1)
			}
		}
	}
	if depth == 0 {
		c.out.WriteString("\n")
	}
}

// renderInline renders the inline content of a node (text + inline markup),
// skipping nested block-level list elements.
func (c *converter) renderInline(n *html.Node) string {
	saved := c.out
	c.out = strings.Builder{}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && (ch.Data == "ul" || ch.Data == "ol") {
			continue // nested lists are handled separately
		}
		c.walk(ch, &listState{})
	}
	result := strings.TrimSpace(c.out.String())
	c.out = saved
	return result
}

// emitCodeBlock renders a <p class="prettyprint"> body, converting <br> to real newlines.
func (c *converter) emitCodeBlock(n *html.Node) {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		switch ch.Type {
		case html.TextNode:
			c.out.WriteString(ch.Data)
		case html.ElementNode:
			if ch.Data == "br" {
				c.out.WriteString("\n")
			} else {
				c.emitCodeBlock(ch)
			}
		}
	}
}

// emitTable renders a <table> as a Markdown pipe table.
func (c *converter) emitTable(n *html.Node, ls *listState) {
	rows := collectRows(n)
	if len(rows) == 0 {
		return
	}

	// Determine column count from the widest row.
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}

	c.out.WriteString("\n")
	for i, row := range rows {
		// Pad row to cols.
		for len(row) < cols {
			row = append(row, "")
		}
		cells := make([]string, cols)
		for j, cell := range row {
			cells[j] = strings.TrimSpace(cell)
		}
		c.out.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if i == 0 {
			// Separator row after header.
			sep := make([]string, cols)
			for j := range sep {
				sep[j] = "---"
			}
			c.out.WriteString("| " + strings.Join(sep, " | ") + " |\n")
		}
	}
}

// collectRows extracts all rows (as string slices) from a table node.
func collectRows(n *html.Node) [][]string {
	var rows [][]string
	var traverse func(*html.Node)
	traverse = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tr" {
			var cells []string
			for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
				if ch.Type == html.ElementNode && (ch.Data == "td" || ch.Data == "th") {
					cells = append(cells, collectCellText(ch))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
			traverse(ch)
		}
	}
	traverse(n)
	return rows
}

// collectCellText extracts plain text from a table cell (strips HTML tags, handles <p>).
func collectCellText(n *html.Node) string {
	var sb strings.Builder
	var traverse func(*html.Node)
	traverse = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode && node.Data == "br" {
			sb.WriteString(" ")
			return
		}
		for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
			traverse(ch)
		}
	}
	traverse(n)
	return strings.TrimSpace(sb.String())
}

// collectText extracts all text content from a subtree (no markup).
func (c *converter) collectText(n *html.Node) string {
	var sb strings.Builder
	var traverse func(*html.Node)
	traverse = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
			return
		}
		for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
			traverse(ch)
		}
	}
	traverse(n)
	return sb.String()
}

// attr returns the value of the named attribute on node n, or "".
func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// hasCSSClass reports whether node n has the given CSS class.
func hasCSSClass(n *html.Node, class string) bool {
	for _, tok := range strings.Fields(attr(n, "class")) {
		if tok == class {
			return true
		}
	}
	return false
}
