package render

import (
	"html/template"
	"slices"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"golang.org/x/net/html"
)

// everyKindOld and everyKindNew hold a block of every kind goldmark parses,
// with one block replaced, one removed and one added between them.
const (
	everyKindOld = "# Title\n\nA paragraph that stays.\n\nA paragraph to replace.\n\n- [ ] task one\n- [x] task two\n\nA paragraph to remove.\n\n| Left | Right |\n|:-----|------:|\n| a    | 1     |\n\n```go\nfunc f() {}\n```\n\n    indented code\n\n> a quote\n\n---\n\n<div>html</div>\n\nA ~~struck~~ word and www.example.com.\n"
	everyKindNew = "# Title\n\nA paragraph that stays.\n\nA paragraph that replaced it.\n\n- [ ] task one\n- [x] task two\n\n| Left | Right |\n|:-----|------:|\n| a    | 1     |\n\n```go\nfunc f() {}\n```\n\n    indented code\n\n> a quote\n\n---\n\n<div>html</div>\n\nA ~~struck~~ word and www.example.com.\n\nAn added paragraph.\n"
)

func TestDocument(t *testing.T) {
	g := NewWithT(t)
	out := Document([]byte(everyKindOld), []byte(everyKindNew), testPage())
	g.Expect(shapesOf(g, out)).To(Equal([]shape{
		{Text: "Title"},
		{Text: "A paragraph that stays."},
		{Mark: markReplaced, Text: "A paragraph that replaced it.", Was: []string{"A paragraph to replace."}},
		{Text: "task one task two"},
		{Mark: markRemoved, Was: []string{"A paragraph to remove."}},
		{Text: "Left Right a 1"},
		{Text: "func f() {}"},
		{Text: "indented code"},
		{Text: "a quote"},
		{Text: ""},
		{Text: "This block is HTML, which is not rendered. Read it in Source."},
		{Text: "A struck word and www.example.com."},
		{Mark: markChanged, Text: "An added paragraph."},
	}))

	// GFM's extensions: a table aligned by attribute, struck text, an
	// autolinked address, and a task box that cannot be ticked.
	body := parseBody(g, out)
	cells := findAll(body, isElement("td"))
	g.Expect(cells).To(HaveLen(2))
	g.Expect(attr(cells[0], "align")).To(Equal("left"))
	g.Expect(attr(cells[1], "align")).To(Equal("right"))
	g.Expect(findAll(body, hasAttr("style"))).To(BeEmpty())
	g.Expect(findAll(body, isElement("del"))).To(HaveLen(1))
	g.Expect(hrefs(body)).To(ContainElement("http://www.example.com"))
	boxes := findAll(body, isElement("input"))
	g.Expect(boxes).To(HaveLen(2))
	for _, box := range boxes {
		g.Expect(hasAttr("disabled")(box)).To(BeTrue())
	}
}

func TestDocument_appendedBlock(t *testing.T) {
	g := NewWithT(t)
	old := "One.\n\nTwo.\n"
	new := "One.\n\nTwo.\n\nThree.\n"
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{"", "", markChanged}))
}

func TestDocument_classesListed(t *testing.T) {
	g := NewWithT(t)
	body := parseBody(g, Document([]byte(everyKindOld), []byte(everyKindNew), testPage()))
	seen := map[string]struct{}{}
	for _, n := range findAll(body, hasAttr("class")) {
		for _, class := range strings.Fields(attr(n, "class")) {
			g.Expect(Classes).To(ContainElement(class), "render writes ."+class+" without listing it")
			seen[class] = struct{}{}
		}
	}
	documentClasses := []string{classBlk, classChanged, classWas, classWasText, classHint, classCueShow, classCueHide, roleKeyword, roleFunction}
	for _, class := range documentClasses {
		g.Expect(seen).To(HaveKey(class), "the fixture renders ."+class)
	}
}

func TestDocument_cues(t *testing.T) {
	g := NewWithT(t)
	body := parseBody(g, Document([]byte(everyKindOld), []byte(everyKindNew), testPage()))
	details := findAll(body, isElement("details"))
	g.Expect(details).NotTo(BeEmpty())
	for _, d := range details {
		summary := elementChildren(d)[0]
		g.Expect(summary.Data).To(Equal("summary"))
		children := elementChildren(summary)
		g.Expect(len(children)).To(BeNumerically(">=", 3))
		show, hide := children[len(children)-2], children[len(children)-1]
		g.Expect(show.Data).To(Equal("span"))
		g.Expect(attr(show, "class")).To(Equal("hint cue-show"))
		g.Expect(textOf(show)).To(Equal("Show what it replaced"))
		g.Expect(hide.Data).To(Equal("span"))
		g.Expect(attr(hide, "class")).To(Equal("hint cue-hide"))
		g.Expect(textOf(hide)).To(Equal("Hide what it replaced"))
	}
}

func TestDocument_cuesOnlyInDetails(t *testing.T) {
	g := NewWithT(t)
	body := parseBody(g, Document([]byte(everyKindOld), []byte(everyKindNew), testPage()))
	for _, block := range elementChildren(body) {
		if block.Data == "details" {
			continue
		}
		g.Expect(findAll(block, hasClass(classCueShow))).To(BeEmpty())
		g.Expect(findAll(block, hasClass(classCueHide))).To(BeEmpty())
	}
	g.Expect(marks(g, Document([]byte(everyKindOld), []byte(everyKindNew), testPage()))).To(ContainElements(markChanged, markRemoved))
}

func TestDocument_emptiedDocument(t *testing.T) {
	g := NewWithT(t)
	g.Expect(shapesOf(g, Document([]byte("One.\n\nTwo.\n"), nil, testPage()))).To(Equal([]shape{
		{Mark: markRemoved, Was: []string{"One.", "Two."}},
	}))
}

func TestDocument_emptyOld(t *testing.T) {
	g := NewWithT(t)
	olds := []string{"", "\n\n", "[r]: x.md\n"}
	for _, old := range olds {
		g.Expect(marks(g, Document([]byte(old), []byte("# New\n\nOne.\n"), testPage()))).To(Equal([]string{"", ""}), "old %q", old)
	}
}

func TestDocument_fencedCode(t *testing.T) {
	g := NewWithT(t)
	md := "```go\nfunc f() int {\n\treturn 1 // one\n}\n```\n\n```nosuchlanguage\nfunc f()\n```\n\n    func f()\n"
	body := parseBody(g, Document(nil, []byte(md), testPage()))
	codes := findAll(body, isElement("code"))
	g.Expect(codes).To(HaveLen(3))
	for _, code := range codes {
		g.Expect(hasAttr("class")(code)).To(BeFalse(), "no language- class, which app.css does not define")
	}
	g.Expect(textOf(codes[0])).To(Equal("func f() int {\n\treturn 1 // one\n}\n"))
	var roles []string
	for _, span := range findAll(codes[0], isElement("span")) {
		roles = append(roles, attr(span, "class")+" "+textOf(span))
	}
	g.Expect(roles).To(Equal([]string{"k func", "f f", "k int", "k return", "n 1", "c // one"}))
	g.Expect(findAll(codes[1], isElement("span"))).To(BeEmpty(), "a fence naming no lexer chroma knows is plain")
	g.Expect(findAll(codes[2], isElement("span"))).To(BeEmpty(), "an indented block is plain")
}

func TestDocument_fencedMiddleLine(t *testing.T) {
	g := NewWithT(t)
	old := "Before.\n\n```\none\ntwo\nthree\n```\n\nAfter one.\n\nAfter two.\n\nAfter three.\n"
	new := strings.Replace(old, "two", "TWO", 1)
	got := shapesOf(g, Document([]byte(old), []byte(new), testPage()))
	g.Expect(got).To(Equal([]shape{
		{Text: "Before."},
		{Mark: markReplaced, Text: "one TWO three", Was: []string{"one two three"}},
		{Text: "After one."},
		{Text: "After two."},
		{Text: "After three."},
	}))
}

func TestDocument_headingIDs(t *testing.T) {
	g := NewWithT(t)
	md := []byte("# Title\n\n## Title\n")
	p := testPage()
	for _, h := range findAll(parseBody(g, Document(nil, md, p)), isElement("h1", "h2")) {
		g.Expect(hasAttr("id")(h)).To(BeFalse())
	}
	p.HeadingIDs = true
	var ids []string
	for _, h := range findAll(parseBody(g, Document(nil, md, p)), isElement("h1", "h2")) {
		ids = append(ids, attr(h, "id"))
	}
	g.Expect(ids).To(Equal([]string{"title", "title-1"}))
}

func TestDocument_htmlBlock(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		was  string
	}{
		{name: "a comment block", old: "<!--\nnote\n-->\n", new: "<!--\nnote two\n-->\n", was: "<!-- note -->"},
		{name: "a div block", old: "<div>\na\n</div>\n", new: "<div>\nb\n</div>\n", was: "<div> a </div>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			doc := "Before.\n\n%s\nAfter.\n"
			old := strings.Replace(doc, "%s", c.old, 1)
			new := strings.Replace(doc, "%s", c.new, 1)
			out := Document([]byte(old), []byte(new), testPage())
			g.Expect(shapesOf(g, out)).To(Equal([]shape{
				{Text: "Before."},
				{Mark: markReplaced, Text: "This block is HTML, which is not rendered. Read it in Source.", Was: []string{c.was}},
				{Text: "After."},
			}))
			g.Expect(hrefs(parseBody(g, out))).To(Equal([]string{"/source"}))
			g.Expect(marks(g, Document([]byte(old), []byte(old), testPage()))).To(Equal([]string{"", "", ""}))
		})
	}
}

func TestDocument_htmlBlockPre(t *testing.T) {
	g := NewWithT(t)
	// The key keeps the whitespace inside any pre element, a raw one with
	// attributes included.
	old := "<pre class=\"x\">\na  b\n</pre>\n"
	new := "<pre class=\"x\">\na b\n</pre>\n"
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{markReplaced}))
}

func TestDocument_htmlBlockUnlinked(t *testing.T) {
	g := NewWithT(t)
	p := testPage()
	p.SourceHref = ""
	body := parseBody(g, Document(nil, []byte("<div>a</div>\n"), p))
	g.Expect(hrefs(body)).To(BeEmpty())
	g.Expect(collapsed(textOf(body))).To(Equal("This block is HTML, which is not rendered."))
}

func TestDocument_images(t *testing.T) {
	cases := []struct {
		name string
		md   string
		// href is where the alt text links, or empty for plain text.
		href string
		text string
	}{
		{name: "a project path", md: "![a diagram](diagram.png)", href: "/f/docs/diagram.png", text: "a diagram"},
		{name: "a root-relative path", md: "![a diagram](/img/d.png)", href: "/f/img/d.png", text: "a diagram"},
		{name: "an external source", md: "![a diagram](https://example.com/d.png)", text: "a diagram"},
		{name: "a fragment", md: "![a diagram](#d)", text: "a diagram"},
		{name: "a refused source", md: "![a diagram](vscode:d.png)", text: "a diagram"},
		{name: "a climb out of the project", md: "![a diagram](../../d.png)", text: "a diagram"},
		{name: "markup in the alt text", md: "![a *big* `diagram`](d.png)", href: "/f/docs/d.png", text: "a big diagram"},
		{name: "inside a link", md: "[![a diagram](d.png)](https://example.com)", href: "https://example.com", text: "a diagram"},
		{name: "inside a refused link", md: "[![a diagram](d.png)](vscode:x)", href: "/f/docs/d.png", text: "a diagram"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			body := parseBody(g, Document(nil, []byte(c.md+"\n"), testPage()))
			g.Expect(findAll(body, isElement("img"))).To(BeEmpty())
			g.Expect(collapsed(textOf(body))).To(Equal(c.text))
			if c.href == "" {
				g.Expect(hrefs(body)).To(BeEmpty())
				return
			}
			g.Expect(hrefs(body)).To(Equal([]string{c.href}))
		})
	}
}

func TestDocument_inlineHTMLChanged(t *testing.T) {
	g := NewWithT(t)
	old := "A <span>x</span> word.\n"
	new := "A <span>y</span> word.\n"
	out := Document([]byte(old), []byte(new), testPage())
	g.Expect(shapesOf(g, out)).To(Equal([]shape{
		{Mark: markReplaced, Text: "A y word.", Was: []string{"A x word."}},
	}))
}

func TestDocument_lineEndings(t *testing.T) {
	g := NewWithT(t)
	// A browser collapses every kind of HTML whitespace, so a file moved
	// from CRLF to LF, or a tab turned into a space, reads the same.
	// goldmark keeps a lone CR, and a form feed, as text.
	old := "One\r\ntwo.\r\n\r\nThree\tfour.\r\n\r\nFive\rsix.\r\n\r\nSeven\feight.\r\n"
	new := "One two.\n\nThree four.\n\nFive six.\n\nSeven eight.\n"
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{"", "", "", ""}))
}

func TestDocument_links(t *testing.T) {
	cases := []struct {
		name string
		md   string
		// href is the link's address, or empty where the text renders
		// with no a element.
		href string
	}{
		{name: "http", md: "[a](http://example.com/x)", href: "http://example.com/x"},
		{name: "https", md: "[a](https://example.com/x?q=1#f)", href: "https://example.com/x?q=1#f"},
		{name: "an upper case scheme", md: "[a](HTTPS://example.com)", href: "HTTPS://example.com"},
		{name: "an address escaped as goldmark writes it", md: "[a](<https://example.com/a b>)", href: "https://example.com/a%20b"},
		{name: "mailto", md: "[a](mailto:x@example.com)", href: "mailto:x@example.com"},
		{name: "a fragment alone", md: "[a](#part)", href: "#part"},
		{name: "javascript", md: "[a](javascript:alert(1))"},
		{name: "javascript in upper case", md: "[a](JAVASCRIPT:alert(1))"},
		{name: "a tab written as an entity", md: "[a](java&#x09;script:alert(1))"},
		{name: "a letter written as an entity", md: "[a](&#106;avascript:alert(1))"},
		{name: "a scheme goldmark does not list", md: "[a](vscode://file/etc/passwd)"},
		{name: "a data image", md: "[a](data:image/png;base64,AAAA)"},
		{name: "file", md: "[a](file:///etc/passwd)"},
		{name: "scheme-relative", md: "[a](//evil.example/x)"},
		{name: "a name starting with a colon", md: "[a](:weird.md)"},
		{name: "a reference to a refused scheme", md: "[a][r]\n\n[r]: vscode:x"},
		{name: "a reference to a path", md: "[a][r]\n\n[r]: other.md", href: "/f/docs/other.md"},
		{name: "an autolink", md: "<https://example.com>", href: "https://example.com"},
		{name: "a refused autolink", md: "<vscode://x>"},
		{name: "an autolink inside a link", md: "[see <https://example.com>](x.md)", href: "/f/docs/x.md"},
		{name: "an autolink inside a refused link", md: "[see <https://example.com>](vscode:x)", href: "https://example.com"},
		{name: "an email autolink", md: "<x@example.com>", href: "mailto:x@example.com"},
		{name: "a bare address", md: "www.example.com", href: "http://www.example.com"},
		{name: "a bare email address", md: "x@example.com", href: "mailto:x@example.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			body := parseBody(g, Document(nil, []byte(c.md+"\n"), testPage()))
			if c.href == "" {
				g.Expect(hrefs(body)).To(BeEmpty())
				g.Expect(textOf(body)).NotTo(BeEmpty(), "the text is still shown")
				return
			}
			g.Expect(hrefs(body)).To(Equal([]string{c.href}))
		})
	}
}

func TestDocument_pairing(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		want []shape
	}{
		{
			name: "an equal run pairs one to one",
			old:  "Kept.\n\nOld one.\n\nOld two.\n",
			new:  "Kept.\n\nNew one.\n\nNew two.\n",
			want: []shape{
				{Text: "Kept."},
				{Mark: markReplaced, Text: "New one.", Was: []string{"Old one."}},
				{Mark: markReplaced, Text: "New two.", Was: []string{"Old two."}},
			},
		},
		{
			name: "an unequal run puts its text on its last new block",
			old:  "Kept.\n\nOld one.\n\nOld two.\n\nOld three.\n\nEnd.\n",
			new:  "Kept.\n\nNew one.\n\nNew two.\n\nEnd.\n",
			want: []shape{
				{Text: "Kept."},
				{Mark: markChanged, Text: "New one."},
				{Mark: markReplaced, Text: "New two.", Was: []string{"Old one.", "Old two.", "Old three."}},
				{Text: "End."},
			},
		},
		{
			name: "a run with more new blocks than old",
			old:  "Kept.\n\nOld one.\n",
			new:  "Kept.\n\nNew one.\n\nNew two.\n",
			want: []shape{
				{Text: "Kept."},
				{Mark: markChanged, Text: "New one."},
				{Mark: markReplaced, Text: "New two.", Was: []string{"Old one."}},
			},
		},
		{
			name: "a removal is its own block",
			old:  "Kept.\n\nGone.\n\nEnd.\n",
			new:  "Kept.\n\nEnd.\n",
			want: []shape{
				{Text: "Kept."},
				{Mark: markRemoved, Was: []string{"Gone."}},
				{Text: "End."},
			},
		},
		{
			name: "an addition replaces nothing",
			old:  "Kept.\n\nEnd.\n",
			new:  "Kept.\n\nNew.\n\nEnd.\n",
			want: []shape{
				{Text: "Kept."},
				{Mark: markChanged, Text: "New."},
				{Text: "End."},
			},
		},
		{
			name: "nothing moved",
			old:  "Kept.\n\nEnd.\n",
			new:  "Kept.\n\nEnd.\n",
			want: []shape{
				{Text: "Kept."},
				{Text: "End."},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(shapesOf(g, Document([]byte(c.old), []byte(c.new), testPage()))).To(Equal(c.want))
		})
	}
}

func TestDocument_pastTheLineLimit(t *testing.T) {
	g := NewWithT(t)
	// Past diff's 20,000 lines the comparison gives no hunks, and the
	// document shows whole with nothing marked rather than empty.
	old := strings.Repeat("a\n\n", 20001)
	got := marks(g, Document([]byte(old), []byte(old+"b\n"), testPage()))
	g.Expect(got).To(HaveLen(20002))
	g.Expect(slices.Compact(got)).To(Equal([]string{""}))
}

func TestDocument_rawHTMLOmitted(t *testing.T) {
	g := NewWithT(t)
	attack := "<script>alert(1)</script>\n\nA paragraph <img src=x onerror=alert(1)> and <a href=\"javascript:x\" onclick=\"alert(1)\">a link</a>.\n\n<style>body { display: none }</style>\n\n- <script>alert(2)</script>\n"
	body := parseBody(g, Document(nil, []byte(attack), testPage()))
	g.Expect(findAll(body, isElement("script"))).To(BeEmpty())
	g.Expect(findAll(body, isElement("img"))).To(BeEmpty())
	g.Expect(findAll(body, isElement("style"))).To(BeEmpty())
	g.Expect(findAll(body, hasAttr("style"))).To(BeEmpty())
	g.Expect(hrefs(body)).To(Equal([]string{"/source", "/source", "/source"}), "the placeholders' links to Source, and no other")
	onAttributes := findAll(body, func(n *html.Node) bool {
		return slices.ContainsFunc(n.Attr, func(a html.Attribute) bool {
			return strings.HasPrefix(a.Key, "on")
		})
	})
	g.Expect(onAttributes).To(BeEmpty())
}

func TestDocument_reflowedParagraph(t *testing.T) {
	g := NewWithT(t)
	old := "A paragraph wrapped\nin one place.\n\nAnother.\n"
	new := "A paragraph\nwrapped in   another.\n\nAnother.\n"
	g.Expect(shapesOf(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]shape{
		{Mark: markReplaced, Text: "A paragraph wrapped in another.", Was: []string{"A paragraph wrapped in one place."}},
		{Text: "Another."},
	}))
	new = "A paragraph\nwrapped in one\nplace.\n\nAnother.\n"
	g.Expect(shapesOf(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]shape{
		{Text: "A paragraph wrapped in one place."},
		{Text: "Another."},
	}))
	// A paragraph between two code blocks in one block is outside both.
	old = "> ```\n> one\n> ```\n>\n> A paragraph wrapped\n> in one place.\n>\n> ```\n> two\n> ```\n"
	new = "> ```\n> one\n> ```\n>\n> A paragraph\n> wrapped in one place.\n>\n> ```\n> two\n> ```\n"
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{""}))
}

func TestDocument_refusedLinkChanged(t *testing.T) {
	g := NewWithT(t)
	// Both sides show the text "here" alone. Only a key holding the
	// destination as written sees the change.
	old := "Go [here](vscode://a).\n"
	new := "Go [here](vscode://b).\n"
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{markReplaced}))
}

func TestDocument_reindentedCode(t *testing.T) {
	g := NewWithT(t)
	old := "Before.\n\n```go\nif x {\n\treturn\n}\n```\n\nAfter.\n"
	new := "Before.\n\n```go\nif x {\n    return\n}\n```\n\nAfter.\n"
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{"", markReplaced, ""}))
}

func TestDocument_relativeLinks(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		md   string
		href string
	}{
		{name: "beside the document", doc: "docs/product.md", md: "[a](architecture.md)", href: "/f/docs/architecture.md"},
		{name: "a dot segment", doc: "docs/product.md", md: "[a](./:weird.md)", href: "/f/docs/:weird.md"},
		{name: "up a directory", doc: "docs/product.md", md: "[a](../README.md)", href: "/f/README.md"},
		{name: "root-relative", doc: "docs/product.md", md: "[a](/internal/x.go)", href: "/f/internal/x.go"},
		{name: "a document at the root", doc: "README.md", md: "[a](docs/x.md)", href: "/f/docs/x.md"},
		{name: "an escaped name", doc: "docs/product.md", md: "[a](sub/50%25%20off.md)", href: "/f/docs/sub/50% off.md"},
		{name: "a fragment kept", doc: "docs/product.md", md: "[a](other.md#part-two)", href: "/f/docs/other.md#part-two"},
		{name: "a query dropped", doc: "docs/product.md", md: "[a](other.md?view=source)", href: "/f/docs/other.md"},
		{name: "an empty destination", doc: "docs/product.md", md: "[a]()", href: "/f/docs/product.md"},
		{name: "a climb out of the project", doc: "docs/product.md", md: "[a](../../etc/passwd)"},
		{name: "a root-relative climb", doc: "docs/product.md", md: "[a](/../etc/passwd)"},
		{name: "a repository's own directory", doc: "docs/product.md", md: "[a](sub/.git/config)"},
		{name: "a NUL", doc: "docs/product.md", md: "[a](a%00b.md)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			p := testPage()
			p.Path = c.doc
			body := parseBody(g, Document(nil, []byte(c.md+"\n"), p))
			if c.href == "" {
				g.Expect(hrefs(body)).To(BeEmpty())
				g.Expect(collapsed(textOf(body))).To(Equal("a"))
				return
			}
			g.Expect(hrefs(body)).To(Equal([]string{c.href}))
		})
	}
}

func TestDocument_repeatedHeading(t *testing.T) {
	g := NewWithT(t)
	p := testPage()
	p.HeadingIDs = true
	old := "## Title\n\nText.\n"
	new := "## Title\n\nNew text.\n\n## Title\n\nText.\n"
	out := Document([]byte(old), []byte(new), p)
	g.Expect(marks(g, out)).To(Equal([]string{markChanged, markChanged, "", ""}))
	var ids []string
	for _, h := range findAll(parseBody(g, out), isElement("h2")) {
		ids = append(ids, attr(h, "id"))
	}
	g.Expect(ids).To(Equal([]string{"title", "title-1"}), "the shown side has ids, so the key could have taken them")
}

func TestDocument_replacedText(t *testing.T) {
	cases := []struct {
		name string
		old  string
		was  string
	}{
		{name: "text nodes", old: "An *emphasized*\n**strong** `code span`\nwith &amp; and \\* and [a link](x.md).", was: "An emphasized strong code span with & and * and a link."},
		{name: "a code span kept raw", old: "Raw `a\\*b`.", was: "Raw a\\*b."},
		{name: "a fenced block's lines", old: "```go\nx := 1\ny := 2\n```", was: "x := 1 y := 2"},
		{name: "an indented block's lines", old: "    x := 1\n    y := 2", was: "x := 1 y := 2"},
		{name: "a table's cells", old: "| a | b |\n|---|---|\n| 1 | 2 |", was: "a b 1 2"},
		{name: "task boxes", old: "- [ ] open\n- [x] done", was: "[ ] open [x] done"},
		{name: "list items", old: "- one\n- two", was: "one two"},
		{name: "an autolink", old: "See <https://example.com>.", was: "See https://example.com."},
		{name: "an image's alt text", old: "An ![alt text](d.png).", was: "An alt text."},
		{name: "inline HTML dropped", old: "A <b>bold</b> word.", was: "A bold word."},
		{name: "a hard line break", old: "One  \ntwo.", was: "One two."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			old := "Kept.\n\n" + c.old + "\n\nEnd.\n"
			new := "Kept.\n\nReplacement.\n\nEnd.\n"
			got := shapesOf(g, Document([]byte(old), []byte(new), testPage()))
			g.Expect(got).To(HaveLen(3))
			g.Expect(got[1].Was).To(Equal([]string{c.was}))
		})
	}
}

func TestIsMarkdown(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{path: "docs/product.md", want: true},
		{path: "notes.markdown", want: true},
		{path: "README.MD", want: true},
		{path: "a.mdx", want: false},
		{path: "a.txt", want: false},
		{path: "md", want: false},
		{path: "docs.md/x.go", want: false},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(IsMarkdown(c.path)).To(Equal(c.want))
		})
	}
}

// The marks a block can carry, as shapesOf reads them.
const (
	markChanged  = "changed"
	markReplaced = "replaced"
	markRemoved  = "removed"
)

// shape is one block of a rendered document as a reader sees it.
type shape struct {
	// Mark is empty for a kept block, or one of the marks.
	Mark string
	// Text is the block's text with its whitespace collapsed, less its cues
	// and its replaced text.
	Text string
	// Was is each replaced block's text.
	Was []string
}

// testPage is a document at docs/product.md whose file pages are /f/ and the
// path, unescaped so a test reads the path the renderer resolved.
func testPage() Page {
	return Page{
		Path:       "docs/product.md",
		Href:       func(p string) string { return "/f/" + p },
		SourceHref: "/source",
	}
}

// shapesOf parses a rendered document and reads each of its blocks.
func shapesOf(g *WithT, out template.HTML) []shape {
	g.THelper()
	var shapes []shape
	for _, block := range elementChildren(parseBody(g, out)) {
		var s shape
		switch class := attr(block, "class"); {
		case block.Data == "details" && class == "blk changed":
			s.Mark = markReplaced
		case block.Data == "div" && class == "blk changed was":
			s.Mark = markRemoved
		case block.Data == "div" && class == "blk changed":
			s.Mark = markChanged
		default:
			g.Expect(block.Data+"."+class).To(Equal("div.blk"), "a block is kept, changed, replaced or removed")
		}
		for _, was := range findAll(block, hasClass(classWasText)) {
			s.Was = append(s.Was, textOf(was))
		}
		s.Text = collapsed(shownText(block))
		shapes = append(shapes, s)
	}
	return shapes
}

// marks is each block's mark.
func marks(g *WithT, out template.HTML) []string {
	g.THelper()
	var out2 []string
	for _, s := range shapesOf(g, out) {
		out2 = append(out2, s.Mark)
	}
	return out2
}

// parseBody parses a rendered document as a page's body.
func parseBody(g *WithT, out template.HTML) *html.Node {
	g.THelper()
	doc, err := html.Parse(strings.NewReader("<!doctype html><html><body>" + string(out) + "</body></html>"))
	g.Expect(err).NotTo(HaveOccurred())
	body := findAll(doc, isElement("body"))
	g.Expect(body).To(HaveLen(1))
	return body[0]
}

// findAll is every node under n, n included, that match accepts.
func findAll(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	if match(n) {
		out = append(out, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, findAll(c, match)...)
	}
	return out
}

// elementChildren is n's child elements.
func elementChildren(n *html.Node) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			out = append(out, c)
		}
	}
	return out
}

// isElement matches an element with one of the names.
func isElement(names ...string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && slices.Contains(names, n.Data)
	}
}

// hasAttr matches an element carrying the attribute.
func hasAttr(key string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && slices.ContainsFunc(n.Attr, func(a html.Attribute) bool {
			return a.Key == key
		})
	}
}

// hasClass matches an element carrying the class.
func hasClass(class string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && slices.Contains(strings.Fields(attr(n, "class")), class)
	}
}

// attr is an attribute's value, or empty.
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// hrefs is the href of every a element under n.
func hrefs(n *html.Node) []string {
	var out []string
	for _, a := range findAll(n, isElement("a")) {
		out = append(out, attr(a, "href"))
	}
	return out
}

// textOf is the text under n as written.
func textOf(n *html.Node) string {
	var b strings.Builder
	for _, t := range findAll(n, func(c *html.Node) bool { return c.Type == html.TextNode }) {
		b.WriteString(t.Data)
	}
	return b.String()
}

// shownText is the text under a block, less its cues and replaced text.
func shownText(n *html.Node) string {
	if hasClass(classWasText)(n) || hasClass(classCueShow)(n) || hasClass(classCueHide)(n) {
		return ""
	}
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(shownText(c))
	}
	// A block element's text is apart from its neighbors', as a browser
	// lays it out, where an inline element's runs on.
	if n.Type == html.ElementNode && !slices.Contains(inlineElements, n.Data) {
		return " " + b.String() + " "
	}
	return b.String()
}

// inlineElements are the elements a document writes inside a line of text.
var inlineElements = []string{"a", "code", "del", "em", "span", "strong"}

// collapsed is s with each run of whitespace one space, and none at the ends.
func collapsed(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
