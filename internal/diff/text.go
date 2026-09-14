package diff

import (
	"bytes"
	"strings"
)

// Op says what happened to one line of a hunk.
type Op string

const (
	// OpContext is a line both sides have, shown for orientation.
	OpContext Op = "context"
	// OpAdd is a line only the new side has.
	OpAdd Op = "add"
	// OpRemove is a line only the old side has.
	OpRemove Op = "remove"
)

// Span is a byte range within a line's text.
type Span struct {
	// Start is the first byte of the range.
	Start int
	// End is one past the last byte.
	End int
}

// Line is one line of a hunk.
type Line struct {
	// Op says which side the line is on.
	Op Op
	// Old is the line's number on the old side, and is zero on an addition.
	Old int
	// New is the line's number on the new side, and is zero on a removal.
	New int
	// Text is the line without its newline.
	Text string
	// NoNewline reports a last line whose file does not end in a newline. It
	// is a property of a real line rather than a line of its own, because
	// rendering it as one puts a line in the file that is not in the file.
	NoNewline bool
	// Marks are the byte ranges within Text that changed, where a run of
	// removals paired one to one with the additions that replaced it.
	Marks []Span
}

// Hunk is a run of changed lines with context around it.
type Hunk struct {
	// OldStart is the first old line number the hunk covers, and OldLines how
	// many. Both are zero where the hunk touches no old line.
	OldStart, OldLines int
	// NewStart is the first new line number the hunk covers, and NewLines how
	// many.
	NewStart, NewLines int
	// Lines is what the hunk shows, in reading order.
	Lines []Line
}

// Result is the comparison of two texts.
type Result struct {
	// Hunks is what changed, with context, and is empty where nothing did.
	Hunks []Hunk
	// Added and Removed count the lines each way. They are what a round's
	// stored size is summed from.
	Added, Removed int
	// Binary reports a file no line diff means anything for.
	Binary bool
	// TooLarge reports a file past the size a diff is taken of. Added and
	// Removed then hold the line count of each side and there are no hunks.
	TooLarge bool
}

// MaxBytes is the largest a side may be before Text stops diffing it.
//
// It is exported so a caller streaming content can stop reading at the point
// the answer stops depending on what follows, rather than holding a file it
// will not diff. Two megabytes is a guess written down so it can be corrected.
const MaxBytes = 2 << 20

// Text compares two texts and returns what moved between them.
func Text(before, after []byte) Result {
	return text(before, after, defaultLimits())
}

// limits are the bounds every bail-out is measured against.
//
// They are a parameter rather than a set of constants so a test can reach each
// bail-out with a handful of lines. A cap only reachable by building a
// twenty-thousand-line fixture is a cap nobody exercises.
type limits struct {
	// maxBytes and maxLines are the size a side is not diffed past.
	maxBytes int
	maxLines int
	// maxTokens is the size a paired line is not word marked past.
	maxTokens int
	// context is how many unchanged lines surround a change.
	context int
}

// defaultLimits are the bounds in use outside the tests.
//
// The two size numbers are guesses written down so they can be corrected. A
// generated file, a minified bundle and a lock file all land past them, and a
// phone rendering forty thousand marked lines is not reading either.
func defaultLimits() limits {
	return limits{
		maxBytes:  MaxBytes,
		maxLines:  20000,
		maxTokens: 400,
		context:   3,
	}
}

// text is Text with its bounds supplied.
func text(before, after []byte, l limits) Result {
	if isBinary(before) || isBinary(after) {
		return Result{Binary: true}
	}
	oldLines, oldEnds := splitLines(before)
	newLines, newEnds := splitLines(after)
	if len(before) > l.maxBytes || len(after) > l.maxBytes || len(oldLines) > l.maxLines || len(newLines) > l.maxLines {
		return Result{TooLarge: true, Added: len(newLines), Removed: len(oldLines)}
	}

	script := compare(oldLines, newLines, oldEnds, newEnds)
	var result Result
	for _, s := range script {
		switch s.op {
		case OpAdd:
			result.Added++
		case OpRemove:
			result.Removed++
		}
	}
	result.Hunks = hunks(script, oldLines, newLines, oldEnds, newEnds, l.context)
	for i := range result.Hunks {
		mark(&result.Hunks[i], l.maxTokens)
	}
	return result
}

// splitLines breaks a text into its lines and reports whether it ended in a
// newline.
//
// The trailing newline is a property of the text rather than a line, so a file
// of one line with no newline and a file of one line with one both hold a
// single line here.
func splitLines(b []byte) ([]string, bool) {
	if len(b) == 0 {
		return nil, true
	}
	s := string(b)
	ends := strings.HasSuffix(s, "\n")
	if ends {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n"), ends
}

// isBinary reports a text no line diff means anything for.
//
// A NUL byte in the first eight kilobytes, which is the test git uses. Deciding
// how a file opens is a different question, and it belongs to internal/kind.
func isBinary(b []byte) bool {
	head := b
	if len(head) > 8000 {
		head = head[:8000]
	}
	return bytes.IndexByte(head, 0) >= 0
}
