package render

import (
	"regexp"
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
