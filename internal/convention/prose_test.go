package convention

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// Test_findingsNameSourceLines pins the offset-to-line map, which is the
// part of reading whole paragraphs that a reader cannot check by eye.
func Test_findingsNameSourceLines(t *testing.T) {
	g := NewWithT(t)
	doc := strings.Join([]string{
		"# A document",
		"",
		"A line with nothing wrong in it.",
		"",
		"A paragraph that starts clean and then says we",
		"somewhere in its second line.",
		"",
		"A later paragraph with an em dash — right here.",
	}, "\n")

	person := prosePerson("a.md", doc)
	g.Expect(person).To(HaveLen(1))
	g.Expect(person[0].At).To(Equal("a.md:5"))

	dashes := proseDashes("a.md", doc)
	g.Expect(dashes).To(HaveLen(1))
	g.Expect(dashes[0].At).To(Equal("a.md:8"))
}

// Test_wrappedSpansStayExempt covers what oxfmt does to a document that
// quotes the thing it forbids. Both patterns stop at a line break, so before the
// lines were joined every one of these was reported.
func Test_wrappedSpansStayExempt(t *testing.T) {
	g := NewWithT(t)
	doc := strings.Join([]string{
		"# A document",
		"",
		"The rule says never to write \"I did this",
		"myself\" in prose, and the quote wraps across the break.",
		"",
		"A code span wraps the same way: `we",
		"are not` and nothing here is a violation.",
		"",
		"An em dash inside a quoted example, \"one —",
		"two\", is quoted material like any other.",
	}, "\n")

	g.Expect(prosePerson("a.md", doc)).To(BeEmpty())
	g.Expect(proseDashes("a.md", doc)).To(BeEmpty())
}
