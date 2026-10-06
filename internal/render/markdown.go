package render

import (
	"html"
	"net/url"
	"path"
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// IsMarkdown reports whether a path names a markdown file, by its extension.
// The case is ignored, so README.MD is markdown as README.md is.
func IsMarkdown(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// Href writes the address of a project path's page. The path has been through
// capture.CleanPath already, or is empty for the project's root, which
// CleanPath refuses since it names no file.
type Href func(projectPath string) string

// Page is what a document needs from the page it is shown on.
type Page struct {
	// Path is the document's project path. A relative link resolves against
	// its directory.
	Path string
	// Href writes the address a link or an image resolving into the project
	// goes to. It must be set.
	Href Href
	// SourceHref is the address of the document's Source view, which an HTML
	// block's placeholder links to. Empty leaves the placeholder unlinked.
	SourceHref string
	// HeadingIDs writes an id on every heading. goldmark numbers a repeated
	// heading's id within one document, so only a page holding one document
	// can have them without repeating an id. A page without them has nothing
	// for a fragment to land on, so a fragment alone goes to the document's
	// own page, through Href.
	HeadingIDs bool
}

// extensions are GFM's, with a table cell's alignment written as an align
// attribute. goldmark's default writes a style attribute, and the surface's
// policy has no style-src, so Chromium ignored it.
func extensions() goldmark.Option {
	return goldmark.WithExtensions(
		extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute)),
		extension.Strikethrough,
		extension.Linkify,
		extension.TaskList,
	)
}

// shownMarkdown parses and renders a document as the page shows it. Raw HTML
// is omitted, because html.WithUnsafe is never given.
//
// It is built for each document rather than shared, since its node renderer
// holds the page's paths.
func shownMarkdown(p Page) goldmark.Markdown {
	var opts []parser.Option
	if p.HeadingIDs {
		opts = append(opts, parser.WithAutoHeadingID())
	}
	// goldmark registers node renderers from the highest priority number to
	// the lowest, so the lowest wins. Its own HTML renderer is 1000.
	return goldmark.New(
		extensions(),
		goldmark.WithParserOptions(opts...),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(shownRenderer{page: p}, 100))),
	)
}

// keyMarkdown parses and renders a block for its key, which is hashed and
// never sent anywhere. So it writes raw HTML and every destination as the
// source wrote it, where the rendering shown would write the same comment or
// the same text for two different blocks.
func keyMarkdown() goldmark.Markdown {
	return goldmark.New(extensions(), goldmark.WithRendererOptions(ghtml.WithUnsafe()))
}

// shownRenderer replaces goldmark's rendering of the nodes that leave the
// document: links and images, which go through the allowlist, and raw HTML
// blocks. It also highlights a fenced code block into the role classes.
type shownRenderer struct {
	page Page
}

// RegisterFuncs registers the overrides.
func (r shownRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindLink, r.link)
	reg.Register(ast.KindAutoLink, r.autoLink)
	reg.Register(ast.KindImage, r.image)
	reg.Register(ast.KindFencedCodeBlock, r.fencedCode)
	reg.Register(ast.KindHTMLBlock, r.htmlBlock)
}

// link writes an a element around a link's text where its destination passes
// the allowlist, and the text alone where it does not.
func (r shownRenderer) link(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	href, kind := r.resolve(node.(*ast.Link).Destination)
	if kind == linkRefused {
		return ast.WalkContinue, nil
	}
	if entering {
		w.WriteString(`<a href="` + html.EscapeString(href) + `">`)
	} else {
		w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

// autoLink writes an autolink as a link to itself, or as its text where the
// allowlist refuses it or it sits inside a link written as an a element, for
// the reason image gives. An autolink has no text children, so its label is
// written here.
func (r shownRenderer) autoLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.AutoLink)
	dest := n.URL(source)
	// An email autolink's URL is the address alone. A written <mailto:x>
	// is a URL autolink, so the scheme is never there already.
	if n.AutoLinkType == ast.AutoLinkEmail {
		dest = append([]byte("mailto:"), dest...)
	}
	label := html.EscapeString(string(n.Label(source)))
	if href, kind := r.resolve(dest); kind != linkRefused && !r.insideLink(n) {
		w.WriteString(`<a href="` + html.EscapeString(href) + `">` + label + `</a>`)
	} else {
		w.WriteString(label)
	}
	return ast.WalkContinue, nil
}

// image writes an image as its alt text, linked to its file page where its
// source resolves into the project. A relative src would ask the file route
// for an image and get a page back, and the policy blocks an external one.
//
// Inside a link that was written as an a element, the alt text stays plain:
// an a inside an a is split in two by the HTML parser.
func (r shownRenderer) image(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	alt := plainText(source, n)
	href, kind := r.resolve(n.Destination)
	if kind == linkProject && !r.insideLink(n) {
		w.WriteString(`<a href="` + html.EscapeString(href) + `">` + alt + `</a>`)
	} else {
		w.WriteString(alt)
	}
	return ast.WalkSkipChildren, nil
}

// insideLink reports whether a node sits inside a link written as an a
// element.
func (r shownRenderer) insideLink(n ast.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if l, ok := p.(*ast.Link); ok {
			if _, kind := r.resolve(l.Destination); kind != linkRefused {
				return true
			}
		}
	}
	return false
}

// fencedCode highlights a fenced code block by its info string, through the
// lexer chroma names for it, into the role classes. goldmark's own rendering
// writes a language- class, which app.css does not define.
func (r shownRenderer) fencedCode(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)
	h := highlighter{lexer: lexers.Get(string(n.Language(source)))}
	w.WriteString("<pre><code>")
	lines := n.Lines()
	for i := range lines.Len() {
		segment := lines.At(i)
		line := strings.TrimSuffix(string(segment.Value(source)), "\n")
		for _, s := range segments(line, h.tokens(line), nil) {
			if s.Role != "" {
				w.WriteString(`<span class="` + s.Role + `">` + html.EscapeString(s.Text) + `</span>`)
			} else {
				w.WriteString(html.EscapeString(s.Text))
			}
		}
		w.WriteString("\n")
	}
	w.WriteString("</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

// htmlBlock writes the placeholder for a block of raw HTML. Raw HTML stays off,
// so goldmark would write a comment in its place and the block would show
// nothing at all.
func (r shownRenderer) htmlBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	w.WriteString(`<p class="` + classHint + `">This block is HTML, which is not rendered.`)
	if r.page.SourceHref != "" {
		w.WriteString(` <a href="` + html.EscapeString(r.page.SourceHref) + `">Read it in Source.</a>`)
	}
	w.WriteString("</p>\n")
	return ast.WalkSkipChildren, nil
}

// linkKind is what the allowlist made of a destination.
type linkKind int

const (
	// linkRefused renders as its text.
	linkRefused linkKind = iota
	// linkAddress is http, https or mailto, written as given.
	linkAddress
	// linkFragment is a fragment alone: written as given where the page
	// writes heading ids, and on the document's own page otherwise.
	linkFragment
	// linkProject is a path resolved into the project.
	linkProject
)

// resolve passes a destination through the allowlist and returns the address
// to write for it, with what kind of destination it is.
//
// goldmark's own check is a denylist of four schemes, and it blanks what it
// refuses to href="", a link to the page that seems to do nothing when tapped.
// A scheme it does not list, such as vscode:, passes it. So the destination is
// parsed and accepted only as http, https or mailto, a fragment alone, or a
// path resolved into the project, its root included.
//
// What is parsed is the destination as goldmark would write it, entities
// decoded and then escaped, so java&#x09;script: is judged as the
// java%09script: that would reach the page.
func (r shownRenderer) resolve(dest []byte) (string, linkKind) {
	escaped := string(util.URLEscape(dest, true))
	u, err := url.Parse(escaped)
	if err != nil {
		return "", linkRefused
	}
	switch {
	case u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "mailto":
		return escaped, linkAddress
	case u.Scheme != "" || u.Host != "":
		// A scheme the list lacks, or a scheme-relative //host, which
		// net/url reads as a host with no scheme.
		return "", linkRefused
	case strings.HasPrefix(escaped, "#") && r.page.HeadingIDs:
		return escaped, linkFragment
	case strings.HasPrefix(escaped, "#"):
		return r.page.Href(r.page.Path) + escaped, linkFragment
	}
	// A relative reference resolves against the document's directory, as a
	// browser would on the document's own file page, and a root-relative one
	// against the project's root. Written as given, it would resolve against
	// whichever page showed the document. The query is dropped, because the
	// file route's query is the surface's own view and comparison.
	target := u.Path
	switch {
	case target == "":
		target = r.page.Path
	case !strings.HasPrefix(target, "/"):
		target = path.Join(path.Dir(r.page.Path), target)
	}
	clean, err := capture.CleanPath(target)
	if err != nil && path.Clean(strings.TrimLeft(target, "/")) == "." {
		// The project's root names no file, so CleanPath refuses it, but the
		// tree has a page for it. A climb past the root cleans to "..", and
		// is still refused.
		clean, err = "", nil
	}
	if err != nil {
		return "", linkRefused
	}
	href := r.page.Href(clean)
	if u.Fragment != "" {
		href += "#" + u.EscapedFragment()
	}
	return href, linkProject
}
