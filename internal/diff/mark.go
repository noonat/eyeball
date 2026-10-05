package diff

import (
	"unicode"
	"unicode/utf8"

	"github.com/aymanbagabas/go-udiff/lcs"
)

// token is one piece of a line, with where it sits in the line's bytes.
type token struct {
	text  string
	start int
	end   int
}

// mark fills in the changed byte ranges within a hunk's changed lines.
//
// Marking runs only where a run of removals pairs one to one with the additions
// that replaced it. Equal-length runs are paired in order. An unequal run is a
// rewrite rather than an edit, and guessing which line replaced which marks the
// wrong words with confidence, which is worse than marking nothing.
func mark(h *Hunk, maxTokens int) {
	lines := h.Lines
	for i := 0; i < len(lines); {
		if lines[i].Op != OpRemove {
			i++
			continue
		}
		// The run of removals, then the additions immediately after it. A
		// removal run with nothing after it is a deletion rather than a
		// replacement, and the lengths cannot match, so it is left alone.
		mid := run(lines, i, OpRemove)
		end := run(lines, mid, OpAdd)
		if mid-i == end-mid {
			for n := 0; n < mid-i; n++ {
				pair(&lines[i+n], &lines[mid+n], maxTokens)
			}
		}
		i = end
	}
}

// run is the index just past a maximal run of one op starting at from.
func run(lines []Line, from int, op Op) int {
	for from < len(lines) && lines[from].Op == op {
		from++
	}
	return from
}

// pair marks what differs between one removed line and the addition that
// replaced it.
//
// The comparison is over tokens rather than bytes, so a changed identifier
// marks as one word instead of as the letters it happens not to share with the
// one before it. Past the cap the pair is left unmarked: the marking is a
// reading aid, and a line long enough to reach it is one nobody reads word by
// word anyway.
func pair(removed, added *Line, maxTokens int) {
	from := tokenize(removed.Text)
	to := tokenize(added.Text)
	if len(from)+len(to) > maxTokens {
		return
	}
	for _, c := range lcs.DiffLines(texts(from), texts(to)) {
		if c.End > c.Start {
			removed.Marks = append(removed.Marks, Span{
				Start: from[c.Start].start,
				End:   from[c.End-1].end,
			})
		}
		if c.ReplEnd > c.ReplStart {
			added.Marks = append(added.Marks, Span{
				Start: to[c.ReplStart].start,
				End:   to[c.ReplEnd-1].end,
			})
		}
	}
}

// texts is the token strings, which is what the comparison is over.
func texts(tokens []token) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		out[i] = t.text
	}
	return out
}

// tokenize splits a line into runs of word characters and runs of everything
// else, whitespace included.
//
// So `foo(a, b)` against `foo(a, c)` marks one token. Splitting on whitespace
// alone would mark the whole call, and splitting per character would mark the
// letters two identifiers happen not to share.
func tokenize(line string) []token {
	var tokens []token
	start := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		next := i + size
		if next < len(line) {
			after, _ := utf8.DecodeRuneInString(line[next:])
			if isWord(r) == isWord(after) {
				i = next
				continue
			}
		}
		tokens = append(tokens, token{text: line[start:next], start: start, end: next})
		start = next
		i = next
	}
	return tokens
}

// isWord reports whether a rune belongs to a name rather than to what separates
// names. An underscore counts, because an identifier is one token or the
// marking is noise.
func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
