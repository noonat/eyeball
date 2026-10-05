package diff

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_compareRebuildsTheNewSide(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 500; i++ {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			g := NewWithT(t)
			before, beforeEnds := splitLines([]byte(randomText(rng)))
			afterText := mutate(rng, strings.Join(before, "\n"))
			after, afterEnds := splitLines([]byte(afterText))

			// The property that says the script is right without a second diff
			// to compare against: replaying it over the old side has to yield
			// the new side exactly, and the removals have to yield the old one.
			script := compare(before, after, beforeEnds, afterEnds)
			var rebuiltNew, rebuiltOld []string
			for _, s := range script {
				switch s.op {
				case OpAdd:
					rebuiltNew = append(rebuiltNew, after[s.new])
				case OpRemove:
					rebuiltOld = append(rebuiltOld, before[s.old])
				default:
					rebuiltNew = append(rebuiltNew, after[s.new])
					rebuiltOld = append(rebuiltOld, before[s.old])
				}
			}
			g.Expect(rebuiltNew).To(Equal(after))
			g.Expect(rebuiltOld).To(Equal(before))
		})
	}
}

func Test_groupMergesChangesWhoseContextOverlaps(t *testing.T) {
	g := NewWithT(t)
	// A change at each end with three unchanged lines between them. Pinned
	// against GNU diff on the same shape, which splits it at -U1 and merges it
	// at -U2, rather than against a second reading of this code.
	script := []step{
		{op: OpRemove},
		{op: OpContext},
		{op: OpContext},
		{op: OpContext},
		{op: OpAdd},
	}

	g.Expect(group(script, 0)).To(Equal([]span{{from: 0, to: 1}, {from: 4, to: 5}}))
	g.Expect(group(script, 1)).To(Equal([]span{{from: 0, to: 2}, {from: 3, to: 5}}))
	g.Expect(group(script, 2)).To(Equal([]span{{from: 0, to: 5}}))
}

func Test_splitLinesReportsTheTrailingNewline(t *testing.T) {
	texts := []struct {
		name  string
		in    string
		lines []string
		ends  bool
	}{
		{
			name:  "empty",
			in:    "",
			lines: nil,
			ends:  true,
		},
		{
			name:  "one line with a newline",
			in:    "a\n",
			lines: []string{"a"},
			ends:  true,
		},
		{
			name:  "one line without one",
			in:    "a",
			lines: []string{"a"},
			ends:  false,
		},
		{
			name:  "just a newline",
			in:    "\n",
			lines: []string{""},
			ends:  true,
		},
		{
			name:  "a blank line in the middle",
			in:    "a\n\nb\n",
			lines: []string{"a", "", "b"},
			ends:  true,
		},
	}
	for _, c := range texts {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)

			lines, ends := splitLines([]byte(c.in))
			g.Expect(lines).To(Equal(c.lines))
			g.Expect(ends).To(Equal(c.ends))
		})
	}
}

func TestText(t *testing.T) {
	cases := []struct {
		name    string
		before  string
		after   string
		added   int
		removed int
	}{
		{
			name:   "identical",
			before: "a\nb\n",
			after:  "a\nb\n",
		},
		{
			name:    "one line changed in the middle",
			before:  "a\nb\nc\n",
			after:   "a\nB\nc\n",
			added:   1,
			removed: 1,
		},
		{
			name:    "an empty old side",
			before:  "",
			after:   "a\nb\n",
			added:   2,
			removed: 0,
		},
		{
			name:    "an empty new side",
			before:  "a\nb\n",
			after:   "",
			added:   0,
			removed: 2,
		},
		{
			name:    "one line replaced by four",
			before:  "a\nb\nc\n",
			after:   "a\nw\nx\ny\nz\nc\n",
			added:   4,
			removed: 1,
		},
		{
			name:    "a word inside a line",
			before:  "keep\nfoo(a, b)\n",
			after:   "keep\nfoo(a, c)\n",
			added:   1,
			removed: 1,
		},
		{
			name:    "two lines replaced by two",
			before:  "one(x)\ntwo(y)\n",
			after:   "one(X)\ntwo(Y)\n",
			added:   2,
			removed: 2,
		},
		{
			name:    "no trailing newline on the new side",
			before:  "a\nb\n",
			after:   "a\nB",
			added:   1,
			removed: 1,
		},
		{
			name:    "only the trailing newline changed",
			before:  "a\n",
			after:   "a",
			added:   1,
			removed: 1,
		},
		{
			name:    "a last line with no newline that did not change",
			before:  "a\nb",
			after:   "x\nb",
			added:   1,
			removed: 1,
		},
		{
			name:    "CRLF is part of the line",
			before:  "a\nb\n",
			after:   "a\r\nb\r\n",
			added:   2,
			removed: 2,
		},
		{
			name:    "one line changed at each end",
			before:  "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n",
			after:   "A\nb\nc\nd\ne\nf\ng\nh\ni\nJ\n",
			added:   2,
			removed: 2,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)

			got := Text([]byte(c.before), []byte(c.after))
			golden(g, c.name, render(got))
			g.Expect(got.Added).To(Equal(c.added))
			g.Expect(got.Removed).To(Equal(c.removed))
			g.Expect(got.Binary).To(BeFalse())
			g.Expect(got.TooLarge).To(BeFalse())
		})
	}
}

func TestText_binary(t *testing.T) {
	g := NewWithT(t)

	// A NUL byte in the first eight kilobytes, which is the test git uses. A
	// line diff of this means nothing, so there is nothing to count either.
	got := Text([]byte("a\n\x00b\n"), []byte("a\nc\n"))
	g.Expect(got.Binary).To(BeTrue())
	g.Expect(got.Hunks).To(BeEmpty())
	g.Expect(got.Added).To(BeZero())
	g.Expect(got.Removed).To(BeZero())
}

func TestText_binaryOnlyPastTheHead(t *testing.T) {
	g := NewWithT(t)
	long := strings.Repeat("a\n", 5000) + "\x00\n"

	// Past the first eight kilobytes the scan does not look, so this reads as
	// text. Reading the whole of a large file to answer the question would
	// cost more than being wrong about it here.
	got := Text([]byte(long), []byte(long))
	g.Expect(got.Binary).To(BeFalse())
}

func TestText_tieBreak(t *testing.T) {
	g := NewWithT(t)

	// Where several placements are equally minimal, something has to pick one,
	// and the pick decides which line an insertion attaches to. This pins what
	// go-udiff picks, so a version bump that changes it is noticed rather than
	// quietly rewriting every diff a reviewer reads. It does not claim the
	// choice is the better reading: GNU diff shifts boundaries with heuristics
	// of its own and lands elsewhere. Which reads better needs real reviews,
	// and is in specs/TODO.md.
	got := Text([]byte("d\nc\nd\na\ne\nd\nd\ne\na\ne\n"), []byte("e\nc\na\na\ne\nd\nd\ne\na\na\ne\n"))
	golden(g, "tie break", render(got))
}

func TestText_tooLarge(t *testing.T) {
	sides := []struct {
		name   string
		limits limits
		before string
		after  string
	}{
		{
			name:   "past the line limit",
			limits: limits{maxBytes: 1 << 20, maxLines: 2, context: 3},
			before: "a\nb\nc\n",
			after:  "a\n",
		},
		{
			name:   "past the byte limit",
			limits: limits{maxBytes: 4, maxLines: 100, context: 3},
			before: "aaaaaaaa\n",
			after:  "a\n",
		},
	}
	for _, c := range sides {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)

			// No hunks, and the counts are the line count of each side, which
			// is the honest answer when the change was never computed.
			got := text([]byte(c.before), []byte(c.after), c.limits)
			g.Expect(got.TooLarge).To(BeTrue())
			g.Expect(got.Hunks).To(BeEmpty())
			g.Expect(got.Removed).To(Equal(len(strings.Split(strings.TrimSuffix(c.before, "\n"), "\n"))))
			g.Expect(got.Added).To(Equal(len(strings.Split(strings.TrimSuffix(c.after, "\n"), "\n"))))
		})
	}
}

// update rewrites the golden files instead of comparing against them.
var update = flag.Bool("update", false, "rewrite the golden files under testdata")

// golden compares a rendered diff against the file holding what it should be.
//
// The expectations are whole diffs, and a whole diff read as a Go string
// literal is unreadable in exactly the way the thing it describes is readable.
// Kept as files, a change to one is reviewed as a diff of a diff.
//
// The risk a golden file carries is that -update makes a wrong answer the new
// expectation. What guards against that here is that the files are read in
// review like any other change, and that planting a fault in the diff has to
// turn a test red rather than rewrite a fixture.
func golden(g *WithT, name, got string) {
	g.THelper()
	path := filepath.Join("testdata", slug(name)+".diff")
	if *update {
		g.Expect(os.MkdirAll("testdata", 0o700)).To(Succeed())
		g.Expect(os.WriteFile(path, []byte(got), 0o600)).To(Succeed())
		return
	}
	want, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal(string(want)))
}

// slug turns a case name into the file name holding its golden output.
func slug(name string) string {
	return strings.ReplaceAll(name, " ", "-")
}

// render writes a result in a compact form the table rows can hold.
//
// Each line is its op, its old and new numbers with a dot where the line is not
// on that side, and its text, with (nonl) on a last line whose file does not
// end in a newline.
func render(r Result) string {
	var b strings.Builder
	for _, h := range r.Hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, line := range h.Lines {
			b.WriteString(marker(line.Op))
			b.WriteString(number(line.Old))
			b.WriteString(" ")
			b.WriteString(number(line.New))
			b.WriteString(" ")
			// A carriage return cannot be written into a golden file without
			// an editor or a checkout normalizing it away.
			b.WriteString(strings.ReplaceAll(marked(line), "\r", `\r`))
			if line.NoNewline {
				b.WriteString("(nonl)")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// marked is a line's text with its changed ranges bracketed, so a golden file
// shows where the marking fell rather than only which lines moved.
func marked(line Line) string {
	var b strings.Builder
	at := 0
	for _, span := range line.Marks {
		b.WriteString(line.Text[at:span.Start])
		b.WriteString("[")
		b.WriteString(line.Text[span.Start:span.End])
		b.WriteString("]")
		at = span.End
	}
	b.WriteString(line.Text[at:])
	return b.String()
}

// marker is the one character an op is written as.
func marker(op Op) string {
	switch op {
	case OpAdd:
		return "+"
	case OpRemove:
		return "-"
	default:
		return " "
	}
}

// number writes a line number, or a dot where the line is not on that side.
func number(n int) string {
	if n == 0 {
		return "."
	}
	return fmt.Sprintf("%d", n)
}

// randomText builds a short text of repeated letters, sometimes without its
// final newline, which is the shape most likely to break a line diff.
func randomText(rng *rand.Rand) string {
	var b strings.Builder
	for i := rng.Intn(12); i > 0; i-- {
		b.WriteRune('a' + rune(rng.Intn(5)))
		b.WriteString("\n")
	}
	s := b.String()
	if rng.Intn(4) == 0 && s != "" {
		return strings.TrimSuffix(s, "\n")
	}
	return s
}

// mutate makes a few edits to a text, so the pair under test is related rather
// than unrelated, which is what a review actually holds.
func mutate(rng *rand.Rand, s string) string {
	lines := strings.Split(s, "\n")
	for k := rng.Intn(4); k > 0; k-- {
		if len(lines) == 0 {
			break
		}
		i := rng.Intn(len(lines))
		letter := string('a' + rune(rng.Intn(5)))
		switch rng.Intn(3) {
		case 0:
			lines = append(lines[:i], lines[i+1:]...)
		case 1:
			lines = append(lines[:i], append([]string{letter}, lines[i:]...)...)
		default:
			lines[i] = letter
		}
	}
	return strings.Join(lines, "\n")
}
