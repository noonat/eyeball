package render

import (
	"fmt"
	"html/template"
	"regexp"
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
		{
			Mark: markMarked,
			Text: "A paragraph that replaced it.",
			Was:  []string{"A paragraph to replace."},
		},
		{Text: "task one"},
		{Text: "task two"},
		{Mark: markRemoved, Was: []string{"A paragraph to remove."}},
		{Text: "Left Right a 1"},
		{Text: "func f() {}"},
		{Text: "indented code"},
		{Text: "a quote"},
		{Text: ""},
		{Text: "This block is HTML, which is not rendered. Read it in Source."},
		{Text: "A struck word and www.example.com."},
		{Mark: markWhole, Text: "An added paragraph."},
	}))

	// GFM's extensions: a table aligned by attribute, struck text, an
	// autolinked address, and a task box that cannot be ticked.
	body := parseBody(g, out)
	cells := findAll(body, isElement("td"))
	g.Expect(cells).To(HaveLen(2))
	g.Expect(attr(cells[0], "align")).To(Equal("left"))
	g.Expect(attr(cells[1], "align")).To(Equal("right"))
	g.Expect(findAll(body, hasAttr("style"))).To(BeEmpty())
	g.Expect(findAll(body, isElement("s"))).To(HaveLen(1))
	g.Expect(hrefs(body)).To(ContainElement("http://www.example.com"))
	boxes := findAll(body, isElement("input"))
	g.Expect(boxes).To(HaveLen(2))
	for _, box := range boxes {
		g.Expect(hasAttr("disabled")(box)).To(BeTrue())
	}
}

// After writes what a page shows under a passage, given its range.
func TestDocument_after(t *testing.T) {
	g := NewWithT(t)
	p := testPage()
	var asked []string
	p.After = func(first, last int) template.HTML {
		asked = append(asked, fmt.Sprintf("%d-%d", first, last))
		return template.HTML(fmt.Sprintf(`<aside data-after="%d"></aside>`, first))
	}
	out := string(Document(nil, []byte("One.\n\n---\n\nTwo.\n"), p))
	g.Expect(asked).To(Equal([]string{"1-1", "5-5"}))
	g.Expect(out).To(MatchRegexp(`(?s)One\..*data-after="1".*<hr>.*Two\..*data-after="5"`))
}

func TestDocument_appendedBlock(t *testing.T) {
	g := NewWithT(t)
	old := "One.\n\nTwo.\n"
	new := "One.\n\nTwo.\n\nThree.\n"
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{"", "", markWhole}))
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
	documentClasses := []string{classBlk, classChanged, classHint, classOld, classWhole, roleKeyword, roleFunction}
	for _, class := range documentClasses {
		g.Expect(seen).To(HaveKey(class), "the fixture renders ."+class)
	}
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
		{Mark: markMarked, Text: "one TWO three", Was: []string{"one two three"}},
		{Text: "After one."},
		{Text: "After two."},
		{Text: "After three."},
	}))
}

// A folded document shows each change with the nearest heading at or above it
// and one passage on each side. Every other run of two or more passages folds
// into one line, and a run of one is shown.
func TestDocument_fold(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		want []string
	}{
		{
			name: "a change deep in a section",
			old:  "# Title\n\nIntro one.\n\nIntro two.\n\n## Section\n\nA.\n\nB.\n\nC.\n\nD.\n\nOld.\n\nE.\n\nF.\n\nG.\n\nOld two.\n\nH.\n\nI.\n\nJ.\n",
			new:  "# Title\n\nIntro one.\n\nIntro two.\n\n## Section\n\nA.\n\nB.\n\nC.\n\nD.\n\nNew.\n\nE.\n\nF.\n\nG.\n\nNew two.\n\nH.\n\nI.\n\nJ.\n",
			want: []string{
				"fold 3: Title / Intro one. / Intro two.",
				"Section",
				"fold 3: A. / B. / C.",
				"D.",
				"marked: New.",
				"E.",
				"F.",
				"G.",
				"marked: New two.",
				"H.",
				"fold 2: I. / J.",
			},
		},
		{
			name: "a removed passage looks above itself for its heading",
			old:  "## One\n\nA.\n\nB.\n\nC.\n\nGone.\n\nD.\n\nE.\n\nF.\n",
			new:  "## One\n\nA.\n\nB.\n\nC.\n\nD.\n\nE.\n\nF.\n",
			want: []string{
				"One",
				"fold 2: A. / B.",
				"C.",
				"removed: Gone.",
				"D.",
				"fold 2: E. / F.",
			},
		},
		{
			name: "a changed heading names its own section",
			old:  "## One\n\nA.\n\nB.\n\n## Two\n\nC.\n\nD.\n",
			new:  "## One\n\nA.\n\nB.\n\n## Second\n\nC.\n\nD.\n",
			want: []string{
				"fold 2: One / A.",
				"B.",
				"marked: Second",
				"C.",
				"D.",
			},
		},
		{
			name: "a document whose rendering did not change",
			old:  "One.\n\nTwo\nwrapped.\n",
			new:  "One.\n\nTwo wrapped.\n",
			want: []string{"fold 2: One. / Two wrapped."},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			p := testPage()
			p.Fold = true
			g.Expect(outline(g, Document([]byte(c.old), []byte(c.new), p))).To(Equal(c.want))
		})
	}
}

// A passage with a comment under it is never folded, so no comment is hidden
// in a fold. After is asked once for every passage with a range, folded or not.
func TestDocument_foldKeepsComments(t *testing.T) {
	g := NewWithT(t)
	old := "Old.\n\nA.\n\nB.\n\nC.\n\nNoted.\n\nD.\n\nE.\n"
	new := "New.\n\nA.\n\nB.\n\nC.\n\nNoted.\n\nD.\n\nE.\n"
	p := testPage()
	p.Fold = true
	var asked []string
	p.After = func(first, last int) template.HTML {
		asked = append(asked, fmt.Sprintf("%d-%d", first, last))
		if first == 9 {
			return `<aside data-after="9"></aside>`
		}
		return ""
	}
	g.Expect(outline(g, Document([]byte(old), []byte(new), p))).To(Equal([]string{
		"marked: New.",
		"A.",
		"fold 2: B. / C.",
		"Noted.",
		"after 9",
		"fold 2: D. / E.",
	}))
	g.Expect(asked).To(Equal([]string{"1-1", "3-3", "5-5", "7-7", "9-9", "11-11", "13-13"}))
}

// An ordered item keeps its own number, so the item after a fold counts on
// from the items folded above it.
func TestDocument_foldNumbersItems(t *testing.T) {
	g := NewWithT(t)
	old := "1. a\n2. b\n3. c\n4. d\n5. e\n6. f\n"
	new := "1. a\n2. b\n3. c\n4. d\n5. changed\n6. f\n"
	p := testPage()
	p.Fold = true
	out := Document([]byte(old), []byte(new), p)
	g.Expect(outline(g, out)).To(Equal([]string{"fold 3: a / b / c", "d", "marked: changed", "f"}))
	var starts []string
	for _, ol := range findAll(parseBody(g, out), isElement("ol")) {
		starts = append(starts, attr(ol, "start"))
	}
	g.Expect(starts).To(Equal([]string{"1", "2", "3", "4", "5", "6"}))
}

// Nothing folds unless asked, nor where nothing was compared: against an
// empty base, or past the comparison's limit.
func TestDocument_foldWhole(t *testing.T) {
	g := NewWithT(t)
	old := "One.\n\nTwo.\n\nThree.\n\nFour.\n"
	new := "One.\n\nTwo.\n\nThree.\n\nChanged.\n"
	g.Expect(outline(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{"One.", "Two.", "Three.", "marked: Changed."}))
	folded := testPage()
	folded.Fold = true
	g.Expect(outline(g, Document([]byte(old), []byte(new), folded))).To(Equal([]string{"fold 2: One. / Two.", "Three.", "marked: Changed."}))
	g.Expect(outline(g, Document(nil, []byte(new), folded))).To(Equal([]string{"One.", "Two.", "Three.", "Changed."}))
	long := strings.Repeat("a\n\n", 20001)
	out := outline(g, Document([]byte(long), []byte(long+"b\n"), folded))
	g.Expect(out).To(HaveLen(20002))
	g.Expect(slices.Compact(out)).To(Equal([]string{"a", "b"}), "every passage shown, none marked")
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
		{
			name: "a comment block",
			old:  "<!--\nnote\n-->\n",
			new:  "<!--\nnote two\n-->\n",
			was:  "<!-- note -->",
		},
		{
			name: "a div block",
			old:  "<div>\na\n</div>\n",
			new:  "<div>\nb\n</div>\n",
			was:  "<div> a </div>",
		},
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
				{
					Mark: markStruck,
					Text: "This block is HTML, which is not rendered. Read it in Source.",
					Was:  []string{c.was},
				},
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
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{markStruck}))
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
		{
			name: "a project path",
			md:   "![a diagram](diagram.png)",
			href: "/f/docs/diagram.png",
			text: "a diagram",
		},
		{
			name: "a root-relative path",
			md:   "![a diagram](/img/d.png)",
			href: "/f/img/d.png",
			text: "a diagram",
		},
		{
			name: "an external source",
			md:   "![a diagram](https://example.com/d.png)",
			text: "a diagram",
		},
		{name: "a fragment", md: "![a diagram](#d)", text: "a diagram"},
		{name: "a refused source", md: "![a diagram](vscode:d.png)", text: "a diagram"},
		{
			name: "a climb out of the project",
			md:   "![a diagram](../../d.png)",
			text: "a diagram",
		},
		{
			name: "markup in the alt text",
			md:   "![a *big* `diagram`](d.png)",
			href: "/f/docs/d.png",
			text: "a big diagram",
		},
		{
			name: "inside a link",
			md:   "[![a diagram](d.png)](https://example.com)",
			href: "https://example.com",
			text: "a diagram",
		},
		{
			name: "inside a refused link",
			md:   "[![a diagram](d.png)](vscode:x)",
			href: "/f/docs/d.png",
			text: "a diagram",
		},
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
		{Mark: markMarked, Text: "A y word.", Was: []string{"A x word."}},
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
		// ids writes heading ids.
		ids bool
		// href is the link's address, or empty where the text renders
		// with no a element.
		href string
		// text is the text shown, where a row checks it.
		text string
	}{
		{name: "http", md: "[a](http://example.com/x)", href: "http://example.com/x"},
		{
			name: "https",
			md:   "[a](https://example.com/x?q=1#f)",
			href: "https://example.com/x?q=1#f",
		},
		{
			name: "an upper case scheme",
			md:   "[a](HTTPS://example.com)",
			href: "HTTPS://example.com",
		},
		{
			name: "an address escaped as goldmark writes it",
			md:   "[a](<https://example.com/a b>)",
			href: "https://example.com/a%20b",
		},
		{name: "mailto", md: "[a](mailto:x@example.com)", href: "mailto:x@example.com"},
		{
			name: "a fragment alone, on a page with heading ids",
			md:   "[a](#part)",
			ids:  true,
			href: "#part",
		},
		{
			name: "a fragment alone, on a page without them",
			md:   "[a](#part)",
			href: "/f/docs/product.md#part",
		},
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
		{
			name: "a reference to a path",
			md:   "[a][r]\n\n[r]: other.md",
			href: "/f/docs/other.md",
		},
		{name: "an autolink", md: "<https://example.com>", href: "https://example.com"},
		{name: "a refused autolink", md: "<vscode://x>"},
		{
			name: "an autolink inside a link",
			md:   "[see <https://example.com>](x.md)",
			href: "/f/docs/x.md",
			text: "see https://example.com",
		},
		{
			name: "an autolink inside a refused link",
			md:   "[see <https://example.com>](vscode:x)",
			href: "https://example.com",
		},
		{name: "an email autolink", md: "<x@example.com>", href: "mailto:x@example.com"},
		{name: "a bare address", md: "www.example.com", href: "http://www.example.com"},
		{
			name: "a bare email address",
			md:   "x@example.com",
			href: "mailto:x@example.com",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			p := testPage()
			p.HeadingIDs = c.ids
			body := parseBody(g, Document(nil, []byte(c.md+"\n"), p))
			if c.text != "" {
				g.Expect(collapsed(textOf(body))).To(Equal(c.text))
			}
			if c.href == "" {
				g.Expect(hrefs(body)).To(BeEmpty())
				g.Expect(textOf(body)).NotTo(BeEmpty(), "the text is still shown")
				return
			}
			g.Expect(hrefs(body)).To(Equal([]string{c.href}))
		})
	}
}

// One changed item marks that item alone. Turning a tight list loose changes
// how every item renders, so it marks every item, and so does turning bullets
// into numbers. Numbering is not part of a key, so an item added at the top of
// an ordered list marks only itself.
func TestDocument_listItemChanged(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		want []string
	}{
		{
			name: "one item changed",
			old:  "- one\n- two\n- three\n",
			new:  "- one\n- second\n- three\n",
			want: []string{"", markMarked, ""},
		},
		{
			name: "a tight list made loose",
			old:  "- one\n- two\n",
			new:  "- one\n\n- two\n",
			want: []string{markWhole, markWhole},
		},
		{
			name: "bullets made numbers",
			old:  "- one\n- two\n",
			new:  "1. one\n2. two\n",
			want: []string{markWhole, markWhole},
		},
		{
			name: "an item added at the top of a numbered list",
			old:  "1. one\n2. two\n",
			new:  "1. zero\n2. one\n3. two\n",
			want: []string{markWhole, "", ""},
		},
		{
			name: "a ticked box",
			old:  "- [ ] one\n- [ ] two\n",
			new:  "- [ ] one\n- [x] two\n",
			want: []string{"", markMarked},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(marks(g, Document([]byte(c.old), []byte(c.new), testPage()))).To(Equal(c.want))
		})
	}
}

// Each item of a top-level list is a passage of its own, written as a list of
// one in its own block with its own lines, so its comments can follow it.
func TestDocument_listItems(t *testing.T) {
	g := NewWithT(t)
	md := "- one\n- two\n  wrapped\n\n3. three\n4. four\n"
	body := parseBody(g, Document(nil, []byte(md), testPage()))
	blocks := elementChildren(body)
	g.Expect(blocks).To(HaveLen(4))
	var got []string
	for _, b := range blocks {
		g.Expect(attr(b, "class")).To(Equal("blk"))
		lists := elementChildren(b)
		g.Expect(lists).To(HaveLen(1))
		items := elementChildren(lists[0])
		g.Expect(items).To(HaveLen(1))
		g.Expect(items[0].Data).To(Equal("li"))
		got = append(got, fmt.Sprintf("%s %s %s %s", attr(b, "data-lines"), lists[0].Data, attr(lists[0], "start"), collapsed(textOf(items[0]))))
	}
	g.Expect(got).To(Equal([]string{
		"1-1 ul  one",
		"2-3 ul  two wrapped",
		"5-5 ol 3 three",
		"6-6 ol 4 four",
	}))
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
				{Mark: markMarked, Text: "New one.", Was: []string{"Old one."}},
				{Mark: markMarked, Text: "New two.", Was: []string{"Old two."}},
			},
		},
		{
			name: "an unequal run puts its old text above its first new block",
			old:  "Kept.\n\nOld one.\n\nOld two.\n\nOld three.\n\nEnd.\n",
			new:  "Kept.\n\nNew one.\n\nNew two.\n\nEnd.\n",
			want: []shape{
				{Text: "Kept."},
				{
					Mark: markWhole,
					Text: "New one.",
					Was:  []string{"Old one.", "Old two.", "Old three."},
				},
				{Mark: markWhole, Text: "New two."},
				{Text: "End."},
			},
		},
		{
			name: "a run with more new blocks than old",
			old:  "Kept.\n\nOld one.\n",
			new:  "Kept.\n\nNew one.\n\nNew two.\n",
			want: []shape{
				{Text: "Kept."},
				{Mark: markWhole, Text: "New one.", Was: []string{"Old one."}},
				{Mark: markWhole, Text: "New two."},
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
				{Mark: markWhole, Text: "New."},
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

// A passage's range is the first and last line of its content in the source,
// read from the parse the page shows.
func TestDocument_ranges(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want string
	}{
		{
			name: "a wrapped paragraph",
			md:   "One line\nwrapped onto two\nand three.\n",
			want: "1-3",
		},
		{
			name: "a nested list in its item",
			md:   "- one\n  - two\n  - three\n- four\n",
			want: "1-3,4-4",
		},
		{
			name: "a fenced block without its fences",
			md:   "```go\nx := 1\ny := 2\n```\n",
			want: "2-3",
		},
		{
			name: "a setext heading without its underline",
			md:   "Title\n=====\n",
			want: "1-1",
		},
		{
			name: "a table",
			md:   "| a | b |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |\n",
			want: "1-4",
		},
		{name: "an HTML block", md: "<div>\nsome\n</div>\n", want: "1-3"},
		{
			name: "after a blank line",
			md:   "First.\n\nSecond\nparagraph.\n",
			want: "1-1,3-4",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			body := parseBody(g, Document(nil, []byte(c.md), testPage()))
			var got []string
			for _, n := range findAll(body, hasAttrNamed("data-lines")) {
				got = append(got, attrOf(n, "data-lines"))
			}
			g.Expect(strings.Join(got, ",")).To(Equal(c.want))
		})
	}
}

// A block with no content line takes no comment, and neither does a removed
// one, which is not in the round's file.
func TestDocument_rangesNone(t *testing.T) {
	g := NewWithT(t)
	empty := []string{"---\n", "```\n```\n"}
	for _, md := range empty {
		body := parseBody(g, Document(nil, []byte(md), testPage()))
		g.Expect(findAll(body, hasAttrNamed("data-lines"))).To(BeEmpty(), md)
	}
	body := parseBody(g, Document([]byte("Kept.\n\nGone.\n"), []byte("Kept.\n"), testPage()))
	g.Expect(findAll(body, hasAttrNamed("data-lines"))).To(HaveLen(1), "the removed block has no range")
}

// A document's own raw HTML is dropped on both sides before words are
// compared, so marking cannot carry it through from either.
func TestDocument_rawHTMLMarked(t *testing.T) {
	g := NewWithT(t)
	old := "A <img src=x onerror=alert(1)> word and <script>alert(1)</script> more.\n\n<div onclick=\"alert(2)\">x</div>\n"
	new := "A <b onmouseover=\"alert(3)\">bold</b> phrase and <script>alert(4)</script> more.\n\n<div onclick=\"alert(5)\">y</div>\n"
	out := Document([]byte(old), []byte(new), testPage())
	g.Expect(marks(g, out)).To(Equal([]string{markMarked, markStruck}))
	body := parseBody(g, out)
	elements := []string{"img", "script", "b", "div"}
	for _, name := range elements {
		g.Expect(findAll(body.FirstChild.FirstChild, isElement(name))).To(BeEmpty(), name)
	}
	g.Expect(string(out)).NotTo(ContainSubstring("alert(1)"), "the old side's script text")
	g.Expect(string(out)).NotTo(ContainSubstring("alert(4)"), "the new side's script text")
	onAttributes := findAll(body, func(n *html.Node) bool {
		return slices.ContainsFunc(n.Attr, func(a html.Attribute) bool {
			return strings.HasPrefix(a.Key, "on")
		})
	})
	g.Expect(onAttributes).To(BeEmpty())
	// The struck HTML block is its source as text, escaped.
	g.Expect(string(out)).To(ContainSubstring(`<del class="old"><code>&lt;div onclick=&quot;alert(2)&quot;&gt;x&lt;/div&gt;</code></del>`))
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
		{
			Mark: markMarked,
			Text: "A paragraph wrapped in another.",
			Was:  []string{"A paragraph wrapped in one place."},
		},
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
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{markChanged}), "the rule alone, since no word changed")
}

func TestDocument_reindentedCode(t *testing.T) {
	g := NewWithT(t)
	old := "Before.\n\n```go\nif x {\n\treturn\n}\n```\n\nAfter.\n"
	new := "Before.\n\n```go\nif x {\n    return\n}\n```\n\nAfter.\n"
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{"", markMarked, ""}))
}

func TestDocument_relativeLinks(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		md   string
		href string
	}{
		{
			name: "beside the document",
			doc:  "docs/product.md",
			md:   "[a](architecture.md)",
			href: "/f/docs/architecture.md",
		},
		{
			name: "a dot segment",
			doc:  "docs/product.md",
			md:   "[a](./:weird.md)",
			href: "/f/docs/:weird.md",
		},
		{
			name: "up a directory",
			doc:  "docs/product.md",
			md:   "[a](../README.md)",
			href: "/f/README.md",
		},
		{
			name: "root-relative",
			doc:  "docs/product.md",
			md:   "[a](/internal/x.go)",
			href: "/f/internal/x.go",
		},
		{
			name: "a document at the root",
			doc:  "README.md",
			md:   "[a](docs/x.md)",
			href: "/f/docs/x.md",
		},
		{
			name: "an escaped name",
			doc:  "docs/product.md",
			md:   "[a](sub/50%25%20off.md)",
			href: "/f/docs/sub/50% off.md",
		},
		{
			name: "a fragment kept",
			doc:  "docs/product.md",
			md:   "[a](other.md#part-two)",
			href: "/f/docs/other.md#part-two",
		},
		{
			name: "a query dropped",
			doc:  "docs/product.md",
			md:   "[a](other.md?view=source)",
			href: "/f/docs/other.md",
		},
		{
			name: "an empty destination",
			doc:  "docs/product.md",
			md:   "[a]()",
			href: "/f/docs/product.md",
		},
		{
			name: "a climb out of the project",
			doc:  "docs/product.md",
			md:   "[a](../../etc/passwd)",
		},
		{
			name: "a root-relative climb",
			doc:  "docs/product.md",
			md:   "[a](/../etc/passwd)",
		},
		{
			name: "a repository's own directory",
			doc:  "docs/product.md",
			md:   "[a](sub/.git/config)",
		},
		{name: "a NUL", doc: "docs/product.md", md: "[a](a%00b.md)"},
		{
			name: "the root, root-relative",
			doc:  "docs/product.md",
			md:   "[a](/)",
			href: "/f/",
		},
		{
			name: "the root, up a directory",
			doc:  "docs/product.md",
			md:   "[a](../)",
			href: "/f/",
		},
		{
			name: "the root, from a document there",
			doc:  "README.md",
			md:   "[a](./)",
			href: "/f/",
		},
		{
			name: "the root with a fragment",
			doc:  "docs/product.md",
			md:   "[a](..#top)",
			href: "/f/#top",
		},
		{name: "a climb past the root", doc: "docs/product.md", md: "[a](../../)"},
		{
			name: "a root-relative climb past the root",
			doc:  "docs/product.md",
			md:   "[a](/../)",
		},
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
	g.Expect(marks(g, out)).To(Equal([]string{markWhole, markWhole, "", ""}))
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
		{
			name: "text nodes",
			old:  "An *emphasized*\n**strong** `code span`\nwith &amp; and \\* and [a link](x.md).",
			was:  "An emphasized strong code span with & and * and a link.",
		},
		{name: "a code span kept raw", old: "Raw `a\\*b`.", was: "Raw a\\*b."},
		{
			name: "a fenced block's lines",
			old:  "```go\nx := 1\ny := 2\n```",
			was:  "x := 1 y := 2",
		},
		{
			name: "an indented block's lines",
			old:  "    x := 1\n    y := 2",
			was:  "x := 1 y := 2",
		},
		{
			name: "a table's cells",
			old:  "| a | b |\n|---|---|\n| 1 | 2 |",
			was:  "a b 1 2",
		},
		{name: "an open task box", old: "- [ ] open", was: "[ ] open"},
		{name: "a ticked task box", old: "- [x] done", was: "[x] done"},
		{name: "a nested list's items", old: "- one\n  - two", was: "one two"},
		{
			name: "an autolink",
			old:  "See <https://example.com>.",
			was:  "See https://example.com.",
		},
		{
			name: "an image's alt text",
			old:  "An ![alt text](d.png).",
			was:  "An alt text.",
		},
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

// A document's own struck text is an s element, so it never reads as a
// removed word.
func TestDocument_strikethrough(t *testing.T) {
	g := NewWithT(t)
	body := parseBody(g, Document(nil, []byte("A ~~struck~~ word.\n"), testPage()))
	g.Expect(findAll(body, isElement("del"))).To(BeEmpty())
	struck := findAll(body, isElement("s"))
	g.Expect(struck).To(HaveLen(1))
	g.Expect(textOf(struck[0])).To(Equal("struck"))
}

// What words cannot mark is shown as the tearout draws it. A removed passage
// is its old text struck through. A rewrite strikes the old text above its
// first new passage and tints each new passage whole. A changed HTML block
// strikes its old source, lines kept, above the placeholder, untinted.
func TestDocument_struck(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		want string
	}{
		{
			name: "a removed passage",
			old:  "Kept.\n\nGone *now*.\n",
			new:  "Kept.\n",
			want: "<div class=\"blk\" data-lines=\"1-1\"><p>Kept.</p></div>\n<div class=\"blk changed\"><del class=\"old\">Gone now.</del>\n</div>\n",
		},
		{
			name: "a removed passage's text, escaped",
			old:  "Kept.\n\nGone `<b>x</b>` & y.\n",
			new:  "Kept.\n",
			want: "<div class=\"blk\" data-lines=\"1-1\"><p>Kept.</p></div>\n<div class=\"blk changed\"><del class=\"old\">Gone &lt;b&gt;x&lt;/b&gt; &amp; y.</del>\n</div>\n",
		},
		{
			name: "one passage made two",
			old:  "Kept.\n\nOne.\n",
			new:  "Kept.\n\nFirst.\n\nSecond.\n",
			want: "<div class=\"blk\" data-lines=\"1-1\"><p>Kept.</p></div>\n<div class=\"blk changed\" data-lines=\"3-3\"><del class=\"old\">One.</del>\n<ins class=\"whole\"><p>First.</p></ins></div>\n<div class=\"blk changed\" data-lines=\"5-5\"><ins class=\"whole\"><p>Second.</p></ins></div>\n",
		},
		{
			name: "a rewrite's old text, escaped",
			old:  "One < two & \"three\".\n",
			new:  "First.\n\nSecond.\n",
			want: "<div class=\"blk changed\" data-lines=\"1-1\"><del class=\"old\">One &lt; two &amp; &quot;three&quot;.</del>\n<ins class=\"whole\"><p>First.</p></ins></div>\n<div class=\"blk changed\" data-lines=\"3-3\"><ins class=\"whole\"><p>Second.</p></ins></div>\n",
		},
		{
			name: "a list item added with nothing replaced",
			old:  "- one\n",
			new:  "- one\n- two\n",
			want: "<div class=\"blk\" data-lines=\"1-1\"><ul>\n<li>one</li>\n</ul></div>\n<div class=\"blk changed\" data-lines=\"2-2\"><ins class=\"whole\"><ul>\n<li>two</li>\n</ul></ins></div>\n",
		},
		{
			name: "a changed HTML block",
			old:  "<div>\na\n</div>\n",
			new:  "<div>\nb\n</div>\n",
			want: "<div class=\"blk changed\" data-lines=\"1-3\"><del class=\"old\"><code>&lt;div&gt;\na\n&lt;/div&gt;</code></del>\n<p class=\"hint\">This block is HTML, which is not rendered. <a href=\"/source\">Read it in Source.</a></p></div>\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(string(Document([]byte(c.old), []byte(c.new), testPage()))).To(Equal(c.want))
		})
	}
}

// Words are marked only where both passages have the same block tags in the
// same order. A table that lost a cell, or a paragraph made a heading, is a
// rewrite, struck above and tinted whole.
func TestDocument_wordBlocks(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
	}{
		{
			name: "a table that lost a column",
			old:  "| a | b |\n|---|---|\n| 1 | 2 |\n",
			new:  "| a |\n|---|\n| 1 |\n",
		},
		{
			name: "a paragraph made a heading",
			old:  "Title words\n",
			new:  "# Title words\n",
		},
		{
			name: "a quote that gained a paragraph",
			old:  "> one two\n",
			new:  "> one\n>\n> two\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			s := shapesOf(g, Document([]byte(c.old), []byte(c.new), testPage()))
			g.Expect(s).To(HaveLen(1))
			g.Expect(s[0].Mark).To(Equal(markWhole))
			g.Expect(s[0].Was).To(HaveLen(1))
		})
	}
}

// A pair of passages is compared word by word up to maxWordTokens between
// them, and shown as a rewrite past it.
func TestDocument_wordLimit(t *testing.T) {
	g := NewWithT(t)
	pair := func(words int) (string, string) {
		return strings.Repeat("word ", words) + "old.\n", strings.Repeat("word ", words) + "new.\n"
	}
	old, new := pair(1240)
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{markMarked}))
	old, new = pair(1260)
	g.Expect(marks(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]string{markWhole}))
}

// Three of the four rules in architecture.md that hold the marking together:
// an entity, whitespace joining and the task box. The fourth, same block tags,
// is TestDocument_wordBlocks.
func TestDocument_wordRules(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		want string
	}{
		{
			name: "an entity is one token",
			old:  "If a < b.",
			new:  "If a > b.",
			want: "<p>If a <del>&lt;</del><ins>&gt;</ins> b.</p>",
		},
		{
			name: "changes separated by whitespace alone join",
			old:  "It gives up.",
			new:  "It records the failure on the job.",
			want: "<p>It <del>gives up</del><ins>records the failure on the job</ins>.</p>",
		},
		{
			name: "a ticked box marks the new box and strikes none",
			old:  "- [ ] Record the base\n",
			new:  "- [x] Record the base\n",
			want: "<ul>\n<li><ins><input checked=\"\" disabled=\"\" type=\"checkbox\"></ins> Record the base</li>\n</ul>",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(markedBody(g, c.old, c.new)).To(Equal(c.want))
		})
	}
}

// Changed words are marked where they stand, in the new passage's own markup:
// added text in ins, and removed text in a del where it was removed, inside
// links, emphasis and code alike.
func TestDocument_words(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		want string
	}{
		{
			name: "a word replaced",
			old:  "The cost is storage.",
			new:  "The cost is small.",
			want: "<p>The cost is <del>storage</del><ins>small</ins>.</p>",
		},
		{
			name: "inside a link",
			old:  "As [the diff](x.md) shows.",
			new:  "As [the whole file](x.md) shows.",
			want: "<p>As <a href=\"/f/docs/x.md\">the <del>diff</del><ins>whole file</ins></a> shows.</p>",
		},
		{
			name: "inside a code span",
			old:  "Name it with `--base-commit`.",
			new:  "Name it with `--base`.",
			want: "<p>Name it with <code>--base<del>-commit</del></code>.</p>",
		},
		{
			name: "an addition at the end of emphasis",
			old:  "**One control.** Then more.",
			new:  "**One control with two settings.** Then more.",
			want: "<p><strong>One control<ins> with two settings</ins>.</strong> Then more.</p>",
		},
		{
			name: "a removal at the end",
			old:  "One two three.",
			new:  "One two.",
			want: "<p>One two<del> three</del>.</p>",
		},
		{
			name: "a removal at the start",
			old:  "Very well done.",
			new:  "Well done.",
			want: "<p><del>Very well</del><ins>Well</ins> done.</p>",
		},
		{
			name: "a change in a list item keeps its number",
			old:  "3. the third\n",
			new:  "3. the last\n",
			want: "<ol start=\"3\">\n<li>the <del>third</del><ins>last</ins></li>\n</ol>",
		},
		{
			name: "a change in a table cell",
			old:  "| a | b |\n|---|---|\n| 1 | 2 |",
			new:  "| a | b |\n|---|---|\n| 1 | 3 |",
			want: "<table>\n<thead>\n<tr>\n<th>a</th>\n<th>b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>1</td>\n<td><del>2</del><ins>3</ins></td>\n</tr>\n</tbody>\n</table>",
		},
		{
			name: "every cell of a row changed",
			old:  "| a | b |\n|---|---|\n| 1 | 2 |",
			new:  "| a | b |\n|---|---|\n| x | y |",
			want: "<table>\n<thead>\n<tr>\n<th>a</th>\n<th>b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td><del>1</del><ins>x</ins></td>\n<td><del>2</del><ins>y</ins></td>\n</tr>\n</tbody>\n</table>",
		},
		{
			name: "a cell emptied",
			old:  "| a | b |\n|---|---|\n| 1 | 2 |",
			new:  "| a | b |\n|---|---|\n|  | 2 |",
			want: "<table>\n<thead>\n<tr>\n<th>a</th>\n<th>b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td><del>1</del></td>\n<td>2</td>\n</tr>\n</tbody>\n</table>",
		},
		{
			name: "a removal after a code block inside a list item",
			old:  "- a\n  ```\n  x\n  ```\n  b\n",
			new:  "- a\n  ```\n  x\n  ```\n",
			want: "<ul>\n<li>a\n<pre><code>x\n</code></pre><del>b</del>\n</li>\n</ul>",
		},
		{
			name: "a code block emptied",
			old:  "```\nx\n```\n",
			new:  "```\n```\n",
			want: "<pre><code><del>x</del></code></pre>",
		},
		{
			name: "a box removed with only a space beside it",
			old:  "- [x] \n",
			new:  "- \n",
			want: "<ul>\n<li></li>\n</ul>",
		},
		{
			name: "formatting alone marks no word",
			old:  "A word made bold.",
			new:  "A word made **bold**.",
			want: "<p>A word made <strong>bold</strong>.</p>",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(markedBody(g, c.old, c.new)).To(Equal(c.want))
		})
	}
}

// The marking copies the new passage through byte for byte, escaped text and
// attributes included, and writes removed text as the renderer escaped it.
// Taking out the marking elements leaves exactly the unmarked rendering.
func TestDocument_wordsEscaped(t *testing.T) {
	g := NewWithT(t)
	old := "Use `<old>` & \"x\" in [a link](https://example.com/?q=1&r=2 \"T&#39;s\") <here>.\n"
	new := "Use `<new>` & \"y\" in [a link](https://example.com/?q=1&r=3 \"T&#39;s\") <there>.\n"
	marked := markedBody(g, old, new)
	plain := strings.TrimSuffix(unmarkedBody(g, new), "\n")
	g.Expect(regexp.MustCompile(`<del>.*?</del>|</?ins>`).ReplaceAllString(marked, "")).To(Equal(plain))
	g.Expect(marked).To(ContainSubstring("<del>old</del>"))
	g.Expect(marked).To(ContainSubstring("&quot;<del>x</del><ins>y</ins>&quot;"))
	g.Expect(marked).To(ContainSubstring(`href="https://example.com/?q=1&amp;r=3"`))
	removed := markedBody(g, "Keep `a < b && c` here.\n", "Keep here.\n")
	g.Expect(removed).To(Equal("<p>Keep<del> a &lt; b &amp;&amp; c</del> here.</p>"), "removed text keeps its escaping and none of its tags")
}

func TestHasBlocks(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   bool
	}{
		{name: "empty", source: "", want: false},
		{name: "whitespace alone", source: "\n  \n\t\n", want: false},
		{
			name:   "a link reference definition alone",
			source: "[r]: other.md\n",
			want:   false,
		},
		{name: "a paragraph", source: "One.\n", want: true},
		{name: "a thematic break", source: "---\n", want: true},
		{name: "an HTML block", source: "<div>\na\n</div>\n", want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(HasBlocks([]byte(c.source))).To(Equal(c.want))
		})
	}
}

// shape is one block of a rendered document as a reader sees it.
type shape struct {
	// Mark is empty for a kept block, or one of the marks.
	Mark string
	// Text is the block's text with its whitespace collapsed, less every
	// removed word and every struck passage: the new text.
	Text string
	// Was is the old text: for a marked block, its text less its added words
	// and with its removed ones, and otherwise each struck passage's text.
	Was []string
}

// markedBody is what a one-passage document marks inside its block, when old
// is replaced by new.
func markedBody(g *WithT, old, new string) string {
	g.THelper()
	return blockBody(g, Document([]byte(old), []byte(new), testPage()))
}

// unmarkedBody is a one-passage document's block as it renders unmarked.
func unmarkedBody(g *WithT, md string) string {
	g.THelper()
	return blockBody(g, Document(nil, []byte(md), testPage()))
}

// oneBlock is a rendering of one block, and what is inside it.
var oneBlock = regexp.MustCompile(`(?s)^<div class="blk[a-z ]*"(?: data-lines="[0-9-]+")?>(.*)</div>\n$`)

// blockBody is what a rendering of one block holds.
func blockBody(g *WithT, out template.HTML) string {
	g.THelper()
	m := oneBlock.FindStringSubmatch(string(out))
	g.Expect(m).To(HaveLen(2), string(out))
	return m[1]
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
		g.Expect(block.Data).To(Equal("div"), "a block is a div")
		old := findAll(block, hasClass(classOld))
		for _, o := range old {
			s.Was = append(s.Was, collapsed(textOf(o)))
		}
		words := findAll(block, func(n *html.Node) bool {
			return (n.Data == "ins" || n.Data == "del") && n.Type == html.ElementNode && attr(n, "class") == ""
		})
		switch class := attr(block, "class"); {
		case class == "blk":
			g.Expect(old).To(BeEmpty())
			g.Expect(words).To(BeEmpty())
		case class != "blk changed":
			g.Expect(class).To(Equal("blk"), "a block is kept or changed")
		case len(findAll(block, hasClass(classWhole))) > 0:
			s.Mark = markWhole
		case len(old) > 0 && hasAttrNamed("data-lines")(block):
			s.Mark = markStruck
		case len(old) > 0:
			s.Mark = markRemoved
		case len(words) > 0:
			s.Mark = markMarked
			s.Was = []string{collapsed(oldText(block))}
		default:
			s.Mark = markChanged
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

// outline reads a rendered document's top level: each passage's mark and
// text, each fold as its count and the passages in it, and what After wrote.
// A fold's count must be the number of passages it holds.
func outline(g *WithT, out template.HTML) []string {
	g.THelper()
	var lines []string
	for _, n := range elementChildren(parseBody(g, out)) {
		switch {
		case n.Data == "details" && attr(n, "class") == classFold:
			children := elementChildren(n)
			g.Expect(children[0].Data).To(Equal("summary"))
			var texts []string
			for _, b := range children[1:] {
				g.Expect(attr(b, "class")).To(Equal("blk"), "a fold holds unchanged passages alone")
				texts = append(texts, collapsed(textOf(b)))
			}
			g.Expect(textOf(children[0])).To(Equal(fmt.Sprintf("%d unchanged passages", len(texts))))
			lines = append(lines, fmt.Sprintf("fold %d: %s", len(texts), strings.Join(texts, " / ")))
		case n.Data == "aside":
			lines = append(lines, "after "+attr(n, "data-after"))
		default:
			s := shapesOf(g, template.HTML(renderHTML(g, n)))
			g.Expect(s).To(HaveLen(1))
			switch s[0].Mark {
			case "":
				lines = append(lines, s[0].Text)
			case markRemoved:
				lines = append(lines, "removed: "+strings.Join(s[0].Was, " / "))
			default:
				lines = append(lines, s[0].Mark+": "+s[0].Text)
			}
		}
	}
	return lines
}

// renderHTML writes a parsed node back out as HTML.
func renderHTML(g *WithT, n *html.Node) string {
	g.THelper()
	var b strings.Builder
	g.Expect(html.Render(&b, n)).To(Succeed())
	return b.String()
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

// shownText is the text under a block, less its removed words and struck
// passages.
func shownText(n *html.Node) string {
	return textLess(n, "del")
}

// oldText is the text a marked block replaced: its text less its added words.
func oldText(n *html.Node) string {
	return textLess(n, "ins")
}

// textLess is the text under n, less every element named skip.
func textLess(n *html.Node, skip string) string {
	if n.Type == html.ElementNode && n.Data == skip {
		return ""
	}
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textLess(c, skip))
	}
	// A block element's text is apart from its neighbors', as a browser
	// lays it out, where an inline element's runs on.
	if n.Type == html.ElementNode && !slices.Contains(inlineElements, n.Data) {
		return " " + b.String() + " "
	}
	return b.String()
}

// inlineElements are the elements a document writes inside a line of text.
var inlineElements = []string{"a", "code", "del", "em", "ins", "s", "span", "strong"}

// collapsed is s with each run of whitespace one space, and none at the ends.
func collapsed(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// hasAttrNamed matches an element carrying an attribute.
func hasAttrNamed(name string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return false
		}
		for _, a := range n.Attr {
			if a.Key == name {
				return true
			}
		}
		return false
	}
}

// attrOf reads an attribute.
func attrOf(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// The marks a block can carry, as shapesOf reads them.
const (
	markChanged = "changed"
	markMarked  = "marked"
	markWhole   = "whole"
	markStruck  = "struck"
	markRemoved = "removed"
)
