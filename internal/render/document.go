package render

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strings"

	"github.com/noonat/eyeball/internal/diff"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

const (
	// keptHTML opens a block that did not change, with its range.
	keptHTML = `<div class="` + classBlk + `"%s>`
	// addedHTML opens a changed block with nothing replaced, with its range.
	addedHTML = `<div class="` + classBlk + ` ` + classChanged + `"%s>`
	// removedHTML opens a removed run with nothing in its place. It is not
	// in the round's file, so it has no range and takes no comment.
	removedHTML = `<div class="` + classBlk + ` ` + classChanged + ` ` + classWas + `">`
	// replacedHTML opens a changed block that holds the text it replaced,
	// with its range.
	replacedHTML = `<details class="` + classBlk + ` ` + classChanged + `"%s><summary>`
	// cuesHTML ends that block's summary with what a tap does, one span for
	// each state. app.css shows one and hides the other by the details
	// element's open attribute, with no script.
	cuesHTML = `<span class="` + classHint + ` ` + classCueShow + `">Show what it replaced</span><span class="` + classHint + ` ` + classCueHide + `">Hide what it replaced</span></summary>`
	// wasTextHTML opens the text of one replaced block.
	wasTextHTML = `<span class="` + classWasText + `">`
	// foldHTML opens a run of unchanged passages folded into one line, with
	// the count it holds. It is a details element, so it opens with no
	// script. A run is two or more, so the count is never one.
	foldHTML = `<details class="` + classFold + `"><summary>%d unchanged passages</summary>` + "\n"
)

// Document renders a markdown document with what changed against old marked
// passage by passage. An old side with no passages marks nothing: an empty
// base and a new file read the same way, and marking every passage marks none.
//
// With p.Fold set, only what changed is shown: each changed passage, the
// nearest heading at or above it, one passage on each side, and any passage
// After writes a comment under. Every other run of two or more passages folds
// into one line that opens in place. A run of one is shown, since its fold line
// would take as much room as the passage. Nothing folds where nothing was
// compared, against an empty base or past the comparison's limit.
func Document(old, new []byte, p Page) template.HTML {
	oldSide, newSide := keyed(old), keyed(new)
	shown := shownMarkdown(p)
	doc := shown.Parser().Parse(text.NewReader(new))
	// The shown parse and the key parse differ only in heading ids, which are
	// attributes on a node, so both find the same passages in order.
	passages := passagesOf(doc)
	starts := lineStarts(new)
	items, compared := plan(oldSide, newSide)
	// Each item is written whole first, comments and all, because whether a
	// passage folds depends on whether After wrote anything under it. After
	// is asked once per passage, since the page counts what it placed.
	written := make([]string, len(items))
	commented := make([]bool, len(items))
	for i, it := range items {
		if it.block < 0 {
			written[i] = removedHTML + wasTexts(it.replaced) + "</div>\n"
			continue
		}
		ps := passages[it.block]
		body := strings.TrimSuffix(ps.render(shown.Renderer(), new, true), "\n")
		first, last := blockRange(ps.node, starts)
		lines := ""
		if first > 0 {
			lines = fmt.Sprintf(` data-lines="%d-%d"`, first, last)
		}
		switch {
		case len(it.replaced) > 0:
			written[i] = fmt.Sprintf(replacedHTML, lines) + body + cuesHTML + wasTexts(it.replaced) + "</details>"
		case it.changed:
			written[i] = fmt.Sprintf(addedHTML, lines) + body + "</div>"
		default:
			written[i] = fmt.Sprintf(keptHTML, lines) + body + "</div>"
		}
		written[i] += "\n"
		if first > 0 && p.After != nil {
			after := string(p.After(first, last))
			written[i] += after
			commented[i] = after != ""
		}
	}
	var visible []bool
	if p.Fold && compared {
		visible = shownItems(items, passages, commented)
	}
	var b strings.Builder
	for i := 0; i < len(items); {
		end := i + 1
		if visible != nil && !visible[i] {
			for end < len(items) && !visible[end] {
				end++
			}
		}
		if end-i < 2 {
			b.WriteString(written[i])
			i = end
			continue
		}
		b.WriteString(fmt.Sprintf(foldHTML, end-i))
		for _, w := range written[i:end] {
			b.WriteString(w)
		}
		b.WriteString("</details>\n")
		i = end
	}
	// Raw HTML is omitted, every destination went through the allowlist, and
	// goldmark escapes text, so the markup is the renderer's own.
	return template.HTML(b.String())
}

// shownItems says which items a folded document shows: every changed item,
// the item on each side of it, the nearest heading at or above it, and every
// item with a comment under it.
func shownItems(items []item, passages []passage, commented []bool) []bool {
	visible := make([]bool, len(items))
	heading := func(i int) bool {
		return items[i].block >= 0 && passages[items[i].block].node.Kind() == ast.KindHeading
	}
	for i, it := range items {
		if commented[i] {
			visible[i] = true
		}
		if !it.changed {
			continue
		}
		for n := max(i-1, 0); n <= min(i+1, len(items)-1); n++ {
			visible[n] = true
		}
		// A removed run is no heading of its own, so its search starts above
		// it. A changed heading names its own section.
		for n := i; n >= 0; n-- {
			if heading(n) {
				visible[n] = true
				break
			}
		}
	}
	return visible
}

// HasBlocks reports whether a document holds any block, by the rule Document
// uses to decide that an old side marks nothing. A file holding only
// whitespace or link reference definitions has none, as an empty one does.
func HasBlocks(source []byte) bool {
	doc := keyMarkdown().Parser().Parse(text.NewReader(source))
	return len(topLevel(doc)) > 0
}

// wasTexts writes one was-text span for each replaced block's text.
func wasTexts(replaced []string) string {
	var b strings.Builder
	for _, t := range replaced {
		b.WriteString(wasTextHTML + t + "</span>")
	}
	return b.String()
}

// side is one version of a document, cut into its top-level blocks, each with
// its key.
type side struct {
	source []byte
	blocks []ast.Node
	keys   []string
}

// keyed parses a document with the key's own goldmark and keys each top-level
// block.
//
// The key is never taken from the rendering shown. goldmark numbers a repeated
// heading's id, so a heading added above another of the same name would
// change the other's id and mark it. A link resolved into the round changes
// address with the page, and raw HTML shown is the same comment whatever it
// held. The parse is separate too, because heading ids are attributes the
// parser writes, and a renderer without them still printed them.
func keyed(source []byte) side {
	md := keyMarkdown()
	doc := md.Parser().Parse(text.NewReader(source))
	s := side{source: source}
	for _, p := range passagesOf(doc) {
		sum := sha256.Sum256([]byte(collapse(p.render(md.Renderer(), source, false))))
		s.blocks = append(s.blocks, p.node)
		s.keys = append(s.keys, hex.EncodeToString(sum[:]))
	}
	return s
}

// passage is one passage of a document: a top-level block, or one item of a
// top-level list.
type passage struct {
	// node is the block, or the list item.
	node ast.Node
	// list is the list an item belongs to, and nil for a block.
	list *ast.List
	// number is an ordered item's number in its list.
	number int
}

// passagesOf cuts a parse into its passages. Each item of a top-level list is a
// passage of its own, so one changed item in a long list marks that item and
// the rest can fold. A nested list stays inside the item that holds it.
func passagesOf(doc ast.Node) []passage {
	var out []passage
	for _, n := range topLevel(doc) {
		l, ok := n.(*ast.List)
		if !ok {
			out = append(out, passage{node: n})
			continue
		}
		number := l.Start
		for it := l.FirstChild(); it != nil; it = it.NextSibling() {
			out = append(out, passage{node: it, list: l, number: number})
			number++
		}
	}
	return out
}

// render writes a passage. An item is written as a list of one, since a
// passage's block and the comments after it are not valid inside a list, and
// goldmark writes an item alone as the li it writes inside its list. numbered
// writes an ordered item's own number as the list's start. A key leaves it
// out, so an item added near the top of a list does not mark every item
// renumbered below it, while the list element still tells a bullet from a
// number.
func (p passage) render(r renderer.Renderer, source []byte, numbered bool) string {
	if p.list == nil {
		return renderNode(r, source, p.node)
	}
	open, end := "<ul>\n", "</ul>\n"
	if p.list.IsOrdered() {
		open, end = "<ol>\n", "</ol>\n"
		if numbered {
			open = fmt.Sprintf("<ol start=\"%d\">\n", p.number)
		}
	}
	return open + renderNode(r, source, p.node) + end
}

// topLevel is a document's blocks: its top-level nodes, less its link
// reference definitions. A definition renders nothing, so as a block it would
// be an empty passage, and a change to one already changes the key of every
// block whose link it resolves.
func topLevel(doc ast.Node) []ast.Node {
	var out []ast.Node
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if n.Kind() != ast.KindLinkReferenceDefinition {
			out = append(out, n)
		}
	}
	return out
}

// lines is the keys as diff.Text's input: one hash to a line, every line
// ending in a newline. A key holding a code block's newlines would span lines,
// and a change on one of them would map to the wrong block. Without the final
// newline an appended block changes the last line's ending, and the block
// before it reads as replaced. A side with no blocks is empty input, where a
// lone newline would read as one empty line.
func (s side) lines() []byte {
	var b strings.Builder
	for _, k := range s.keys {
		b.WriteString(k + "\n")
	}
	return []byte(b.String())
}

// renderNode renders one node alone. No node renderer here or in goldmark
// returns an error, and a strings.Builder cannot fail a write, so there is no
// error to report.
func renderNode(r renderer.Renderer, source []byte, n ast.Node) string {
	var b strings.Builder
	_ = r.Render(&b, source, n)
	return b.String()
}

var (
	// preHTML is a pre element, whose whitespace a key keeps, since an
	// indentation change in a code block is a change.
	preHTML = regexp.MustCompile(`(?s)<pre[\s>].*?</pre>`)
	// whitespace is HTML's whitespace, which a browser collapses.
	whitespace = regexp.MustCompile(`[ \t\n\f\r]+`)
)

// collapse turns each run of whitespace outside a pre element into one space.
// goldmark writes a soft line break as a newline, so two wrappings of one
// paragraph render differently until it is collapsed.
func collapse(s string) string {
	var (
		b  strings.Builder
		at int
	)
	for _, m := range preHTML.FindAllStringIndex(s, -1) {
		b.WriteString(whitespace.ReplaceAllString(s[at:m[0]], " "))
		b.WriteString(s[m[0]:m[1]])
		at = m[1]
	}
	b.WriteString(whitespace.ReplaceAllString(s[at:], " "))
	return b.String()
}

// item is one block of the rendering: a block of the new side, or a removed
// run with nothing in its place.
type item struct {
	// block is the new side's block, or -1 for a removed run.
	block int
	// changed marks the block.
	changed bool
	// replaced is the text of each old block it replaced, as HTML.
	replaced []string
}

// plan compares two sides by their keys and says how each block is shown, and
// whether the two were compared at all.
//
// The pairing is internal/diff's word-marking rule. A run of removed blocks
// followed by as many added ones pairs one to one, and each added block holds
// the text of the one it replaced. An unequal run is a rewrite, and pairing
// across it would claim to know which passage replaced which, so each block of
// it is marked and the whole run's replaced text goes on its last new block,
// read after the new passage.
func plan(old, new side) ([]item, bool) {
	items := make([]item, len(new.keys))
	for i := range items {
		items[i].block = i
	}
	if len(old.keys) == 0 {
		return items, false
	}
	// diff.Whole gives every line in one hunk, or no hunk where nothing
	// moved. Past diff's line limit it gives none either, and the document
	// shows with nothing marked and nothing compared.
	r := diff.Whole(old.lines(), new.lines())
	if r.TooLarge {
		return items, false
	}
	if len(r.Hunks) == 0 {
		return items, true
	}
	items = items[:0]
	lines := r.Hunks[0].Lines
	for i := 0; i < len(lines); {
		switch lines[i].Op {
		case diff.OpContext:
			items = append(items, item{block: lines[i].New - 1})
			i++
			continue
		case diff.OpAdd:
			items = append(items, item{block: lines[i].New - 1, changed: true})
			i++
			continue
		}
		mid := run(lines, i, diff.OpRemove)
		end := run(lines, mid, diff.OpAdd)
		removed, added := lines[i:mid], lines[mid:end]
		switch {
		case len(added) == 0:
			items = append(items, item{block: -1, changed: true, replaced: old.texts(removed)})
		case len(added) == len(removed):
			for n, a := range added {
				items = append(items, item{block: a.New - 1, changed: true, replaced: old.texts(removed[n : n+1])})
			}
		default:
			for _, a := range added {
				items = append(items, item{block: a.New - 1, changed: true})
			}
			items[len(items)-1].replaced = old.texts(removed)
		}
		i = end
	}
	return items, true
}

// run is the index just past a run of one op starting at from.
func run(lines []diff.Line, from int, op diff.Op) int {
	for from < len(lines) && lines[from].Op == op {
		from++
	}
	return from
}

// texts is the replaced text of each removed line's block.
func (s side) texts(removed []diff.Line) []string {
	out := make([]string, 0, len(removed))
	for _, l := range removed {
		out = append(out, plainText(s.source, s.blocks[l.Old-1]))
	}
	return out
}

// plainText is the text a node holds without its markup, as escaped HTML with
// its whitespace collapsed.
//
// Text nodes alone miss most of it. A code block holds lines and no text
// nodes, an autolink holds its label, and an HTML block holds its source,
// which is what a changed one opens to since nothing of it is rendered. A
// task item's box is written as [ ] or [x], or ticking it would read as no
// change. A space goes before every block, so two table cells or two list
// items do not run together.
func plainText(source []byte, n ast.Node) string {
	var b strings.Builder
	w := bufio.NewWriter(&b)
	writer := ghtml.DefaultWriter
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if c.Type() == ast.TypeBlock {
			w.WriteByte(' ')
		}
		switch c := c.(type) {
		case *ast.Text:
			// A text segment still holds its backslash escapes and
			// entities, which the writer resolves before escaping, except
			// in a code span, whose text is raw.
			if c.IsRaw() {
				writer.RawWrite(w, c.Segment.Value(source))
			} else {
				writer.Write(w, c.Segment.Value(source))
			}
			if c.SoftLineBreak() || c.HardLineBreak() {
				w.WriteByte(' ')
			}
		case *ast.AutoLink:
			writer.RawWrite(w, c.Label(source))
		case *east.TaskCheckBox:
			if c.IsChecked {
				w.WriteString("[x] ")
			} else {
				w.WriteString("[ ] ")
			}
		case *ast.HTMLBlock:
			writeLines(w, source, c.Lines())
			// goldmark keeps the closing line of HTML block types 1 to 5,
			// such as a comment's -->, out of Lines. A type 6 block, such
			// as a div, has no closure, and reading ClosureLine on it
			// panicked.
			if c.HasClosure() {
				writer.RawWrite(w, c.ClosureLine.Value(source))
			}
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock, *ast.FencedCodeBlock, *frontMatter:
			writeLines(w, source, c.Lines())
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	w.Flush()
	return strings.TrimSpace(whitespace.ReplaceAllString(b.String(), " "))
}

// writeLines writes a block's source lines, escaped.
func writeLines(w *bufio.Writer, source []byte, lines *text.Segments) {
	for i := range lines.Len() {
		line := lines.At(i)
		ghtml.DefaultWriter.RawWrite(w, line.Value(source))
	}
}

// lineStarts is the byte offset each line of source starts at, so an offset
// maps to its line by a search.
func lineStarts(source []byte) []int {
	starts := []int{0}
	for i, c := range source {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// lineOf is the one-based line holding a byte offset.
func lineOf(starts []int, offset int) int {
	return sort.Search(len(starts), func(i int) bool { return starts[i] > offset })
}

// blockRange is the first and last source line of any content beneath a
// top-level block, or zeros for a block with none.
//
// It is read from the parse the page shows, so it cannot disagree with the
// block a reader taps. goldmark's segments leave out a fenced block's fences
// and a setext heading's underline, so a comment on either quotes only its
// text, and a thematic break or an empty fenced block has no content line and
// takes no comment.
func blockRange(n ast.Node, starts []int) (int, int) {
	first, last := 0, 0
	note := func(seg text.Segment) {
		if seg.Stop <= seg.Start {
			return
		}
		a, b := lineOf(starts, seg.Start), lineOf(starts, seg.Stop-1)
		if first == 0 || a < first {
			first = a
		}
		if b > last {
			last = b
		}
	}
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if c.Type() == ast.TypeBlock {
			lines := c.Lines()
			for i := 0; i < lines.Len(); i++ {
				note(lines.At(i))
			}
		}
		if t, ok := c.(*ast.Text); ok {
			note(t.Segment)
		}
		return ast.WalkContinue, nil
	})
	return first, last
}
