package render

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/noonat/eyeball/internal/diff"
)

// File is one file of a diff as the review page prints it.
type File struct {
	// Rows are the hunk headers and lines, in reading order. There are none
	// for a binary or too-large file.
	Rows []Row
	// Binary and TooLarge say why a file has no rows.
	Binary, TooLarge bool
}

// Row is one row of a diff table: a hunk header, or a line.
type Row struct {
	// Header is a hunk header's text, and is empty on a line.
	Header string
	// Class is the row's class: add, del, or empty for context.
	Class string
	// Number is the line's number: the old side's on a removed line, the new
	// side's otherwise.
	Number int
	// Sign is +, the minus sign, or empty for context.
	Sign string
	// Segments are the line's text, cut where a role or a word mark starts or
	// ends.
	Segments []Segment
	// NoNewline says the line is the last of a file with no final newline.
	NoNewline bool
}

// Segment is a run of a line with one role and one mark state.
//
// A segment with both renders as a mark span around a role span. Cutting the
// line at every boundary of either kind is what keeps the two nested: a mark
// crossing a token boundary becomes two segments, each wholly inside one token.
type Segment struct {
	// Text is the run's text.
	Text string
	// Role is the run's syntax class, one of the role classes, or empty.
	Role string
	// Mark says the run changed, within a line paired with its replacement.
	Mark bool
}

// Diff turns a comparison of one file into rows, highlighting each line by the
// lexer path's name chooses.
func Diff(path string, r diff.Result) File {
	return rows(path, r, true)
}

// Whole turns diff.Whole's one hunk into rows with no hunk header. The header
// would say the hunk covers every line, which is what the page already is.
func Whole(path string, r diff.Result) File {
	return rows(path, r, false)
}

// Plain turns a file's content into rows with no comparison: every line
// unchanged, highlighted, and numbered from one. A binary file and one past
// the diff's limits have no rows, as in a comparison.
func Plain(path string, content []byte) File {
	return rows(path, diff.Plain(content), false)
}

// rows turns a result's hunks into rows, each headed by its hunk header where
// headers is set.
func rows(path string, r diff.Result, headers bool) File {
	if r.Binary || r.TooLarge {
		return File{Binary: r.Binary, TooLarge: r.TooLarge}
	}
	h := newHighlighter(path)
	var f File
	for _, hunk := range r.Hunks {
		if headers {
			f.Rows = append(f.Rows, Row{Header: hunkHeader(hunk)})
		}
		for _, line := range hunk.Lines {
			row := Row{Number: line.New, NoNewline: line.NoNewline}
			switch line.Op {
			case diff.OpAdd:
				row.Class, row.Sign = classAdd, "+"
			case diff.OpRemove:
				row.Class, row.Sign, row.Number = classDel, "−", line.Old
			}
			row.Segments = segments(line.Text, h.tokens(line.Text), line.Marks)
			f.Rows = append(f.Rows, row)
		}
	}
	return f
}

// hunkHeader is a hunk's range in git's form, which leaves out a count of one.
func hunkHeader(h diff.Hunk) string {
	return fmt.Sprintf("@@ -%s +%s @@", hunkRange(h.OldStart, h.OldLines), hunkRange(h.NewStart, h.NewLines))
}

// hunkRange is one side of a hunk header.
func hunkRange(start, lines int) string {
	if lines == 1 {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d,%d", start, lines)
}

// segments cuts text at every token and mark boundary.
func segments(text string, tokens []token, marks []diff.Span) []Segment {
	cuts := map[int]bool{0: true, len(text): true}
	for _, t := range tokens {
		cuts[t.start] = true
		cuts[t.end] = true
	}
	for _, m := range marks {
		cuts[m.Start] = true
		cuts[m.End] = true
	}
	points := make([]int, 0, len(cuts))
	for c := range cuts {
		if c >= 0 && c <= len(text) {
			points = append(points, c)
		}
	}
	sort.Ints(points)
	// Tokens and marks are each in order and do not overlap, so one cursor
	// into each walks them with the cuts. A scan of both for every segment
	// would grow with the square of a long line's length.
	var (
		out    []Segment
		ti, mi int
	)
	for i := 0; i+1 < len(points); i++ {
		start, end := points[i], points[i+1]
		for ti < len(tokens) && tokens[ti].end <= start {
			ti++
		}
		for mi < len(marks) && marks[mi].End <= start {
			mi++
		}
		seg := Segment{Text: text[start:end]}
		if ti < len(tokens) && tokens[ti].start <= start {
			seg.Role = tokens[ti].role
		}
		seg.Mark = mi < len(marks) && marks[mi].Start <= start
		out = append(out, seg)
	}
	return out
}
