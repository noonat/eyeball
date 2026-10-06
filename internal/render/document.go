package render

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"regexp"
	"strings"

	"github.com/noonat/eyeball/internal/diff"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

const (
	// keptHTML opens a block that did not change.
	keptHTML = `<div class="` + classBlk + `">`
	// addedHTML opens a changed block with nothing replaced.
	addedHTML = `<div class="` + classBlk + ` ` + classChanged + `">`
	// removedHTML opens a removed run with nothing in its place.
	removedHTML = `<div class="` + classBlk + ` ` + classChanged + ` ` + classWas + `">`
	// replacedHTML opens a changed block that holds the text it replaced.
	replacedHTML = `<details class="` + classBlk + ` ` + classChanged + `"><summary>`
	// cuesHTML ends that block's summary with what a tap does, one span for
	// each state. app.css shows one and hides the other by the details
	// element's open attribute, with no script.
	cuesHTML = `<span class="` + classHint + ` ` + classCueShow + `">Show what it replaced</span><span class="` + classHint + ` ` + classCueHide + `">Hide what it replaced</span></summary>`
	// wasTextHTML opens the text of one replaced block.
	wasTextHTML = `<span class="` + classWasText + `">`
)

// Document renders a markdown document with what changed against old marked
// block by block. An old side with no blocks marks nothing: an empty base and
// a new file read the same way, and marking every passage marks none.
func Document(old, new []byte, p Page) template.HTML {
	oldSide, newSide := keyed(old), keyed(new)
	shown := shownMarkdown(p)
	doc := shown.Parser().Parse(text.NewReader(new))
	// The shown parse and the key parse differ only in heading ids, which are
	// attributes on a node, so both find the same top-level blocks in order.
	var blocks []string
	for _, n := range topLevel(doc) {
		blocks = append(blocks, strings.TrimSuffix(renderNode(shown.Renderer(), new, n), "\n"))
	}
	var b strings.Builder
	for _, it := range plan(oldSide, newSide) {
		switch {
		case it.block < 0:
			b.WriteString(removedHTML + wasTexts(it.replaced) + "</div>")
		case len(it.replaced) > 0:
			b.WriteString(replacedHTML + blocks[it.block] + cuesHTML + wasTexts(it.replaced) + "</details>")
		case it.changed:
			b.WriteString(addedHTML + blocks[it.block] + "</div>")
		default:
			b.WriteString(keptHTML + blocks[it.block] + "</div>")
		}
		b.WriteString("\n")
	}
	// Raw HTML is omitted, every destination went through the allowlist, and
	// goldmark escapes text, so the markup is the renderer's own.
	return template.HTML(b.String())
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
	for _, n := range topLevel(doc) {
		sum := sha256.Sum256([]byte(collapse(renderNode(md.Renderer(), source, n))))
		s.blocks = append(s.blocks, n)
		s.keys = append(s.keys, hex.EncodeToString(sum[:]))
	}
	return s
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

// plan compares two sides by their keys and says how each block is shown.
//
// The pairing is internal/diff's word-marking rule. A run of removed blocks
// followed by as many added ones pairs one to one, and each added block holds
// the text of the one it replaced. An unequal run is a rewrite, and pairing
// across it would claim to know which passage replaced which, so each block of
// it is marked and the whole run's replaced text goes on its last new block,
// read after the new passage.
func plan(old, new side) []item {
	items := make([]item, len(new.keys))
	for i := range items {
		items[i].block = i
	}
	if len(old.keys) == 0 {
		return items
	}
	// diff.Whole gives every line in one hunk, or no hunk where nothing
	// moved. Past diff's line limit it gives none either, and the document
	// shows with nothing marked.
	r := diff.Whole(old.lines(), new.lines())
	if len(r.Hunks) == 0 {
		return items
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
	return items
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
		case *ast.CodeBlock, *ast.FencedCodeBlock:
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
