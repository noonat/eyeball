package render

import (
	"bytes"
	"slices"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// kindFrontMatter is the node kind of a document's front matter.
var kindFrontMatter = ast.NewNodeKind("FrontMatter")

// frontMatter is the YAML between a document's first line and the next line
// that is also ---. Its lines include both delimiters, so its range covers
// them and an empty one still takes a comment.
type frontMatter struct {
	ast.BaseBlock
}

// Kind is kindFrontMatter.
func (n *frontMatter) Kind() ast.NodeKind {
	return kindFrontMatter
}

// IsRaw says the lines are kept as written rather than parsed as inlines.
func (n *frontMatter) IsRaw() bool {
	return true
}

// Dump writes the node for goldmark's debugging output.
func (n *frontMatter) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// frontMatterParser opens a frontMatter on a document's first line.
type frontMatterParser struct{}

// isDelimiter reports whether a line is ---, allowing trailing whitespace and
// a CRLF ending.
func isDelimiter(line []byte) bool {
	return string(util.TrimRightSpace(line)) == "---"
}

// Trigger is the byte a front matter's first line starts with.
func (frontMatterParser) Trigger() []byte {
	return []byte{'-'}
}

// Open opens a frontMatter only on line 0, and only when a later line closes
// it. A block parser cannot give back the lines it has taken, so an unclosed
// one would swallow the rest of the document. The source ahead is scanned
// first, and without a closing line the --- stays a thematic break.
func (frontMatterParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	if n, _ := reader.Position(); n != 0 {
		return nil, parser.NoChildren
	}
	line, segment := reader.PeekLine()
	if !isDelimiter(line) {
		return nil, parser.NoChildren
	}
	if !slices.ContainsFunc(bytes.SplitAfter(reader.Source()[segment.Stop:], []byte("\n")), isDelimiter) {
		return nil, parser.NoChildren
	}
	n := &frontMatter{}
	n.Lines().Append(segment)
	reader.AdvanceToEOL()
	return n, parser.NoChildren
}

// Continue takes each line up to and including the closing ---.
func (frontMatterParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, segment := reader.PeekLine()
	if line == nil {
		return parser.Close
	}
	node.Lines().Append(segment)
	reader.AdvanceToEOL()
	if isDelimiter(line) {
		return parser.Close
	}
	return parser.Continue | parser.NoChildren
}

// Close does nothing, since the lines were taken as they came.
func (frontMatterParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
}

// CanInterruptParagraph is false. Front matter is only ever the first line.
func (frontMatterParser) CanInterruptParagraph() bool {
	return false
}

// CanAcceptIndentedLine is false, since an indented --- is not a delimiter.
func (frontMatterParser) CanAcceptIndentedLine() bool {
	return false
}

// frontMatterExtension adds the parser to both goldmarks, so the shown parse
// and the key parse find the same blocks. goldmark tries block parsers from
// the lowest priority number, and its thematic break is 200.
type frontMatterExtension struct{}

// Extend registers the parser.
func (frontMatterExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithBlockParsers(util.Prioritized(frontMatterParser{}, 50)))
}

// keyFrontMatter writes a front matter's key as its source lines, escaped, in
// a pre element so collapse keeps its whitespace. goldmark's renderer panics
// with an index out of range on a node kind it has no function for, so the key
// renderer needs one as the shown renderer does.
type keyFrontMatter struct{}

// RegisterFuncs registers the key rendering.
func (keyFrontMatter) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindFrontMatter, func(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			w.WriteString("<pre>")
			lines := node.Lines()
			for i := range lines.Len() {
				segment := lines.At(i)
				w.Write(util.EscapeHTML(segment.Value(source)))
			}
			w.WriteString("</pre>\n")
		}
		return ast.WalkSkipChildren, nil
	})
}
