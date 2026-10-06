package render

import (
	"regexp"
	"strings"
	"testing"

	"github.com/noonat/eyeball"
	"github.com/noonat/eyeball/internal/diff"
	. "github.com/onsi/gomega"
)

func Test_classesAreInTheStylesheet(t *testing.T) {
	g := NewWithT(t)
	css := string(eyeball.Stylesheet)
	for _, class := range Classes {
		selector := regexp.MustCompile(`\.` + regexp.QuoteMeta(class) + `([^\w-]|$)`)
		g.Expect(selector.MatchString(css)).To(BeTrue(), "app.css has no ."+class)
	}
	// The table covers every role the highlighter can choose, and every
	// class a diff's rows carry.
	for _, r := range roles {
		g.Expect(Classes).To(ContainElement(r.class))
	}
	f := Diff("a.go", diff.Text([]byte("return 1\n"), []byte("return 2\n")))
	g.Expect(f.Rows).NotTo(BeEmpty())
	for _, row := range f.Rows {
		if row.Class != "" {
			g.Expect(Classes).To(ContainElement(row.Class))
		}
	}
}

func Test_hunkHeaderInGitsForm(t *testing.T) {
	cases := []struct {
		hunk diff.Hunk
		want string
	}{
		{hunk: diff.Hunk{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1}, want: "@@ -1 +1 @@"},
		{hunk: diff.Hunk{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 3}, want: "@@ -0,0 +1,3 @@"},
		{hunk: diff.Hunk{OldStart: 4, OldLines: 2, NewStart: 4, NewLines: 1}, want: "@@ -4,2 +4 @@"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(hunkHeader(c.hunk)).To(Equal(c.want))
		})
	}
}

func Test_segmentsNestAMarkCrossingATokenBoundary(t *testing.T) {
	g := NewWithT(t)
	// A mark that starts inside one token and ends inside the next is cut at
	// the boundary, so each piece sits wholly inside one role.
	text := "s.capture(ctx, base, id)"
	tokens := []token{{start: 2, end: 9, role: roleFunction}}
	marks := []diff.Span{{Start: 5, End: 14}}
	g.Expect(segments(text, tokens, marks)).To(Equal([]Segment{
		{Text: "s."},
		{Text: "cap", Role: roleFunction},
		{Text: "ture", Role: roleFunction, Mark: true},
		{Text: "(ctx,", Mark: true},
		{Text: " base, id)"},
	}))
}

func TestDiff(t *testing.T) {
	g := NewWithT(t)
	before := "package a\n\nfunc f() int {\n\treturn 1\n}\n"
	after := "package a\n\nfunc f() int {\n\treturn 2\n}\nvar x = 3"
	got := Diff("a.go", diff.Text([]byte(before), []byte(after)))

	g.Expect(got.Binary).To(BeFalse())
	g.Expect(got.TooLarge).To(BeFalse())
	type shape struct {
		Header    string
		Class     string
		Number    int
		Sign      string
		Text      string
		NoNewline bool
	}
	var shapes []shape
	for _, r := range got.Rows {
		text := ""
		for _, s := range r.Segments {
			text += s.Text
		}
		shapes = append(shapes, shape{Header: r.Header, Class: r.Class, Number: r.Number, Sign: r.Sign, Text: text, NoNewline: r.NoNewline})
	}
	g.Expect(shapes).To(Equal([]shape{
		{Header: "@@ -1,5 +1,6 @@"},
		{Number: 1, Text: "package a"},
		{Number: 2, Text: ""},
		{Number: 3, Text: "func f() int {"},
		{Class: "del", Number: 4, Sign: "−", Text: "\treturn 1"},
		{Class: "add", Number: 4, Sign: "+", Text: "\treturn 2"},
		{Number: 5, Text: "}"},
		{Class: "add", Number: 6, Sign: "+", Text: "var x = 3", NoNewline: true},
	}))
	// The changed word is marked inside its role.
	g.Expect(got.Rows[4].Segments).To(Equal([]Segment{
		{Text: "\t"},
		{Text: "return", Role: roleKeyword},
		{Text: " "},
		{Text: "1", Role: roleNumber, Mark: true},
	}))
}

func TestDiff_noRows(t *testing.T) {
	g := NewWithT(t)
	g.Expect(Diff("a.png", diff.Result{Binary: true})).To(Equal(File{Binary: true}))
	g.Expect(Diff("a.go", diff.Result{TooLarge: true, Added: 30000})).To(Equal(File{TooLarge: true}))
}

func TestPlain(t *testing.T) {
	g := NewWithT(t)
	got := Plain("a.go", []byte("package a\n\nfunc f() {}"))
	g.Expect(got.Binary).To(BeFalse())
	g.Expect(got.TooLarge).To(BeFalse())
	g.Expect(rowShapes(got)).To(Equal([]rowShape{
		{Number: 1, Text: "package a"},
		{Number: 2, Text: ""},
		{Number: 3, Text: "func f() {}", NoNewline: true},
	}))
	// Each line is highlighted by the file's lexer, as a diff's are.
	g.Expect(got.Rows[2].Segments).To(ContainElement(Segment{Text: "func", Role: roleKeyword}))
	g.Expect(Plain("a.go", []byte("a\x00b"))).To(Equal(File{Binary: true}))
	g.Expect(Plain("a.go", []byte(strings.Repeat("x\n", 20001)))).To(Equal(File{TooLarge: true}))
}

func TestWhole(t *testing.T) {
	g := NewWithT(t)
	// Changes eight lines apart, so a diff splits them into two hunks and
	// leaves the unchanged ends out.
	before := "package a\n\nvar a = 1\n\n// one\n// two\n// three\n// four\n// five\n// six\n// seven\nvar b = 1\n\n// end\n// end\n// end\n// end\n"
	after := strings.Replace(strings.Replace(before, "var a = 1", "var a = 2", 1), "var b = 1", "var b = 2", 1)
	r := diff.Whole([]byte(before), []byte(after))
	got := Whole("a.go", r)

	shapes := rowShapes(got)
	g.Expect(shapes).To(HaveLen(19), "every line of the file, the two removed lines among them")
	for _, s := range shapes {
		g.Expect(s.Header).To(BeEmpty(), "the whole file carries no hunk header")
	}
	g.Expect(shapes[0]).To(Equal(rowShape{Number: 1, Text: "package a"}))
	g.Expect(shapes[2]).To(Equal(rowShape{Class: "del", Number: 3, Sign: "−", Text: "var a = 1"}))
	g.Expect(shapes[3]).To(Equal(rowShape{Class: "add", Number: 3, Sign: "+", Text: "var a = 2"}))
	g.Expect(shapes[18]).To(Equal(rowShape{Number: 17, Text: "// end"}))
	// The word marks are the comparison's own.
	g.Expect(got.Rows[3].Segments).To(ContainElement(Segment{Text: "2", Role: roleNumber, Mark: true}))
	g.Expect(Whole("a.go", diff.Result{Binary: true})).To(Equal(File{Binary: true}))
	g.Expect(Whole("a.go", diff.Result{TooLarge: true})).To(Equal(File{TooLarge: true}))
}

// rowShape is a row with its segments joined, for a test comparing rows.
type rowShape struct {
	Header    string
	Class     string
	Number    int
	Sign      string
	Text      string
	NoNewline bool
}

// rowShapes is every row of f as a shape.
func rowShapes(f File) []rowShape {
	var out []rowShape
	for _, r := range f.Rows {
		text := ""
		for _, s := range r.Segments {
			text += s.Text
		}
		out = append(out, rowShape{Header: r.Header, Class: r.Class, Number: r.Number, Sign: r.Sign, Text: text, NoNewline: r.NoNewline})
	}
	return out
}
