package render

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestDocument_frontMatter(t *testing.T) {
	g := NewWithT(t)
	md := "---\nstatus: draft\nrounds: 2\nrequired_acks:\n  - reviewed\n---\n\n# Title\n"
	out := Document(nil, []byte(md), testPage())
	g.Expect(shapesOf(g, out)).To(Equal([]shape{
		{Text: "--- status: draft rounds: 2 required_acks: - reviewed ---"},
		{Text: "Title"},
	}))
	body := parseBody(g, out)
	codes := findAll(body, isElement("code"))
	g.Expect(codes).To(HaveLen(1))
	g.Expect(textOf(codes[0])).To(Equal("---\nstatus: draft\nrounds: 2\nrequired_acks:\n  - reviewed\n---\n"))
	// A key and a plain value match none of the roles, so the number is the
	// one span.
	var roles []string
	for _, span := range findAll(codes[0], isElement("span")) {
		roles = append(roles, attr(span, "class")+" "+textOf(span))
	}
	g.Expect(roles).To(Equal([]string{"n 2"}))
	g.Expect(findAll(body, isElement("hr", "h2", "ul"))).To(BeEmpty())
}

func TestDocument_frontMatterChanged(t *testing.T) {
	g := NewWithT(t)
	old := "---\nstatus: draft\n---\n\n# Title\n"
	new := strings.Replace(old, "draft", "ready", 1)
	g.Expect(shapesOf(g, Document([]byte(old), []byte(new), testPage()))).To(Equal([]shape{
		{Mark: markReplaced, Text: "--- status: ready ---", Was: []string{"--- status: draft ---"}},
		{Text: "Title"},
	}))
}

func TestDocument_frontMatterRanges(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want string
	}{
		{name: "both delimiters", md: "---\na: 1\nb: 2\n---\n\n# T\n", want: "1-4,6-6"},
		{name: "empty", md: "---\n---\n# T\n", want: "1-2,3-3"},
		{name: "CRLF endings", md: "---\r\na: 1\r\n---\r\n# T\r\n", want: "1-3,4-4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			body := parseBody(g, Document(nil, []byte(c.md), testPage()))
			g.Expect(findAll(body, isElement("pre"))).To(HaveLen(1))
			var got []string
			for _, n := range findAll(body, hasAttrNamed("data-lines")) {
				got = append(got, attrOf(n, "data-lines"))
			}
			g.Expect(strings.Join(got, ",")).To(Equal(c.want))
		})
	}
}

// Each of these renders as it did before front matter was read: no code block.
func TestDocument_notFrontMatter(t *testing.T) {
	cases := []struct {
		name string
		md   string
	}{
		{name: "unclosed", md: "---\nstatus: draft\n\n# Title\n\nBody.\n"},
		{name: "below the first line", md: "Intro.\n\n---\na: b\n---\n"},
		{name: "closed by ...", md: "---\na: 1\n...\n# T\n"},
		{name: "after a byte order mark", md: "\xef\xbb\xbf---\na: 1\n---\n# T\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			body := parseBody(g, Document(nil, []byte(c.md), testPage()))
			g.Expect(findAll(body, isElement("pre"))).To(BeEmpty())
		})
	}
	g := NewWithT(t)
	g.Expect(shapesOf(g, Document(nil, []byte(cases[0].md), testPage()))).To(Equal([]shape{
		{Text: ""},
		{Text: "status: draft"},
		{Text: "Title"},
		{Text: "Body."},
	}), "an unclosed one leaves the rest of the document as it was")
}
