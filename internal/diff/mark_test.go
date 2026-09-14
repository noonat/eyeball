package diff

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_markLeavesAnUnequalRunAlone(t *testing.T) {
	g := NewWithT(t)

	// One line became three. Which of the three replaced it is a guess, and
	// pairing across the run would mark the wrong words with confidence.
	got := Text([]byte("keep\nalpha\n"), []byte("keep\nalpha one\nalpha two\nalpha three\n"))
	for _, line := range got.Hunks[0].Lines {
		g.Expect(line.Marks).To(BeEmpty())
	}
}

func Test_markPairsAnEqualRunInOrder(t *testing.T) {
	g := NewWithT(t)

	// Three for three, so each addition is the line that replaced the removal
	// in the same position. Pairing them in any other order would mark whole
	// lines here rather than the one word that moved in each.
	got := Text(
		[]byte("alpha one\nbeta two\ngamma three\n"),
		[]byte("alpha ONE\nbeta TWO\ngamma THREE\n"),
	)
	lines := got.Hunks[0].Lines
	g.Expect(lines).To(HaveLen(6))
	want := []string{"one", "two", "three", "ONE", "TWO", "THREE"}
	for i, w := range want {
		g.Expect(lines[i].Marks).To(HaveLen(1))
		span := lines[i].Marks[0]
		g.Expect(lines[i].Text[span.Start:span.End]).To(Equal(w))
	}
}

func Test_markWholeLineChanged(t *testing.T) {
	g := NewWithT(t)

	// Nothing in common, so the mark covers the line. Whether that is worth
	// suppressing below some proportion of shared tokens is in specs/TODO.md.
	got := Text([]byte("keep\nalpha\n"), []byte("keep\nomega\n"))
	lines := got.Hunks[0].Lines
	g.Expect(lines[1].Marks).To(Equal([]Span{{Start: 0, End: len("alpha")}}))
	g.Expect(lines[2].Marks).To(Equal([]Span{{Start: 0, End: len("omega")}}))
}

func Test_pairPastTheTokenCapIsLeftUnmarked(t *testing.T) {
	g := NewWithT(t)
	before := "keep\n" + strings.Repeat("word ", 40) + "end\n"
	after := "keep\n" + strings.Repeat("word ", 40) + "END\n"

	// The marking is a reading aid, and a line long enough to reach the cap is
	// not one anybody reads word by word.
	tight := limits{maxBytes: 1 << 20, maxLines: 100, maxTokens: 8, context: 3}
	capped := text([]byte(before), []byte(after), tight)
	for _, line := range capped.Hunks[0].Lines {
		g.Expect(line.Marks).To(BeEmpty())
	}

	// The same pair under a cap it fits in marks the one word that changed.
	roomy := limits{maxBytes: 1 << 20, maxLines: 100, maxTokens: 400, context: 3}
	marked := text([]byte(before), []byte(after), roomy)
	g.Expect(marked.Hunks[0].Lines[1].Marks).To(HaveLen(1))
}

func Test_tokenizeSplitsWordsFromEverythingElse(t *testing.T) {
	lines := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "a call",
			in:   "foo(a, b)",
			want: []string{"foo", "(", "a", ", ", "b", ")"},
		},
		{
			name: "an underscore holds an identifier together",
			in:   "read_file = 1",
			want: []string{"read_file", " = ", "1"},
		},
		{
			name: "leading whitespace is its own token",
			in:   "  x",
			want: []string{"  ", "x"},
		},
		{
			name: "empty",
			in:   "",
			want: []string{},
		},
		{
			name: "a rune wider than a byte",
			in:   "é=2",
			want: []string{"é", "=", "2"},
		},
	}
	for _, c := range lines {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)

			got := tokenize(c.in)
			g.Expect(texts(got)).To(Equal(c.want))
			// The offsets have to name the bytes the token came from, since
			// they become the spans a renderer highlights.
			for _, tok := range got {
				g.Expect(c.in[tok.start:tok.end]).To(Equal(tok.text))
			}
		})
	}
}
