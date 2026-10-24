package render

import (
	"regexp"
	"slices"
	"strings"

	"github.com/noonat/eyeball/internal/diff"
)

// maxWordTokens is the size of a pair of passages past which their words are
// not compared, and the change is shown as a rewrite instead. The comparison
// is quadratic at worst, and a passage that long, such as a long code block,
// is not read word by word. 5,000 is a guess written down so it can be
// corrected.
const maxWordTokens = 5000

// blockTags are the elements a passage's structure is made of. Words are
// marked only where both passages hold the same ones in the same order,
// because removed text is written where it was removed, and in a table that
// lost a cell that point is between two cells, where text is not valid.
var blockTags = []string{
	"blockquote", "dd", "div", "dl", "dt", "h1", "h2", "h3", "h4", "h5", "h6",
	"hr", "li", "ol", "p", "pre", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul",
}

// entity is a character reference in goldmark's escaped text.
var entity = regexp.MustCompile(`^&(#[0-9]+|#[xX][0-9a-fA-F]+|[a-zA-Z][a-zA-Z0-9]*);`)

// piece is one part of a rendered passage: a tag, or a token of its text.
type piece struct {
	// html is the piece as the renderer wrote it.
	html string
	// cmp is what the comparison reads, or empty for a tag it skips. A task
	// box is a tag the comparison reads, as [ ] or [x].
	cmp string
	// tag reports a tag, or whitespace that only lays out block tags, which
	// is copied through and never marked.
	tag bool
	// inline reports a tag of an inline element, such as code or a.
	inline bool
	// seg is the stretch of the passage the piece sits in: the text between
	// one block tag and the next, opening or closing, numbered in order from
	// 0 before the first. Two passages with the same block tags have the same
	// stretches, so a word removed from one has a place in the other.
	seg int
	// starts is the stretch a block tag begins, or -1 for any other piece.
	starts int
	// depth is how many inline tags come before the piece in its stretch.
	depth int
}

// markWords marks the words a passage changed against the one it replaced.
// Both are rendered HTML, cut into tags and text. Only the text is compared,
// by the tokens a changed code line uses. The new passage's tags are copied
// through where they stand, added text is wrapped in ins, and the removed text
// is written in a del at the point it was removed. ok is false where the two
// do not share their block structure, or are too long to compare, and the
// change is a rewrite.
//
// Everything written is the renderer's own output, already escaped, so the
// marking adds no markup beyond its two elements.
func markWords(old, new string) (string, bool) {
	from, fromShape := cut(old)
	to, toShape := cut(new)
	if !slices.Equal(fromShape, toShape) {
		return "", false
	}
	fromTok, toTok := compared(from), compared(to)
	if len(fromTok)+len(toTok) > maxWordTokens {
		return "", false
	}
	changes := joinSpaces(diff.Compare(cmps(from, fromTok), cmps(to, toTok)), cmps(to, toTok))

	inserted := make([]bool, len(toTok))
	before := map[int][]string{}
	after := map[int][]string{}
	lone := map[placed][]string{}
	for _, c := range changes {
		for k := c.To; k < c.ToEnd; k++ {
			inserted[k] = true
		}
		for _, g := range removedText(from, fromTok[c.From:c.FromEnd]) {
			j := c.To
			last, first := -1, -1
			for k := j - 1; k >= 0 && to[toTok[k]].seg >= g.seg; k-- {
				if to[toTok[k]].seg == g.seg {
					last = k
					break
				}
			}
			for k := j; k < len(toTok) && to[toTok[k]].seg <= g.seg; k++ {
				if to[toTok[k]].seg == g.seg {
					first = k
					break
				}
			}
			switch {
			case c.ToEnd > c.To && first == j:
				// A replacement's removed words go just before the words
				// that replaced them.
				before[j] = append(before[j], g.text)
			case last >= 0:
				after[last] = append(after[last], g.text)
			case first >= 0:
				before[first] = append(before[first], g.text)
			default:
				// The stretch the words were removed from holds no new
				// text, so they go where they stood in it: after as many of
				// its inline tags as came before them, such as inside an
				// emptied code block's code element.
				// With no word beside it, whitespace at either end of it
				// would be struck as a space of its own, and whitespace alone
				// as an empty del, a thin tinted sliver with nothing in it.
				text := strings.TrimSpace(g.text)
				if text == "" {
					continue
				}
				at := placed{seg: g.seg, depth: g.depth}
				lone[at] = append(lone[at], text)
			}
		}
	}

	var b strings.Builder
	insOpen := false
	closeIns := func() {
		if insOpen {
			b.WriteString("</ins>")
			insOpen = false
		}
	}
	dels := func(texts []string) {
		if len(texts) == 0 {
			return
		}
		closeIns()
		for _, t := range texts {
			b.WriteString("<del>" + t + "</del>")
		}
	}
	tokenAt := make(map[int]int, len(toTok))
	for j, pi := range toTok {
		tokenAt[pi] = j
	}
	// A stretch's lone removed text goes after its depth-th inline tag, or at
	// the stretch's end where the new stretch holds fewer.
	depths := map[int]int{}
	for _, p := range to {
		if p.inline {
			depths[p.seg]++
		}
	}
	lonely := func(seg, depth int) {
		var here []placed
		for at := range lone {
			if at.seg == seg && min(at.depth, depths[seg]) == depth {
				here = append(here, at)
			}
		}
		slices.SortFunc(here, func(a, b placed) int { return a.depth - b.depth })
		for _, at := range here {
			dels(lone[at])
		}
	}
	lonely(0, 0)
	for pi, p := range to {
		j, isToken := tokenAt[pi]
		if !isToken {
			closeIns()
			b.WriteString(p.html)
			switch {
			case p.starts >= 0:
				lonely(p.starts, 0)
			case p.inline:
				lonely(p.seg, p.depth+1)
			}
			continue
		}
		dels(before[j])
		if inserted[j] {
			if !insOpen {
				b.WriteString("<ins>")
				insOpen = true
			}
		} else {
			closeIns()
		}
		b.WriteString(p.html)
		dels(after[j])
	}
	closeIns()
	return b.String(), true
}

// removed is removed text bound for one stretch, with how many inline tags
// came before it there.
type removed struct {
	seg   int
	depth int
	text  string
}

// placed is where a stretch's lone removed text goes.
type placed struct {
	seg   int
	depth int
}

// removedText is the text of a run of removed tokens, split where it crosses
// from one stretch into the next, so each part is written in its own.
// A removed task box is not written: struck through beside the new box it would
// read as a second box.
func removedText(from []piece, run []int) []removed {
	var out []removed
	for _, pi := range run {
		p := from[pi]
		if p.tag {
			continue
		}
		if len(out) == 0 || out[len(out)-1].seg != p.seg {
			out = append(out, removed{seg: p.seg, depth: p.depth})
		}
		out[len(out)-1].text += p.html
	}
	return out
}

// joinSpaces merges two changes separated only by whitespace. Kept as a match,
// a lone space cuts "gives up" against "records the failure on the job" into
// two pairs of marks, which reads worse than one.
func joinSpaces(changes []diff.Change, to []string) []diff.Change {
	var out []diff.Change
	for _, c := range changes {
		if n := len(out); n > 0 && onlySpace(to[out[n-1].ToEnd:c.To]) {
			out[n-1].FromEnd, out[n-1].ToEnd = c.FromEnd, c.ToEnd
			continue
		}
		out = append(out, c)
	}
	return out
}

// onlySpace reports whether a run of tokens is whitespace and nothing else.
// An empty run is not, since two changes touching are already one.
func onlySpace(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, t := range tokens {
		if strings.TrimSpace(t) != "" {
			return false
		}
	}
	return true
}

// cut splits a rendered passage into its pieces, and returns its shape: the
// block elements it opens and closes, in order.
//
// goldmark escapes every attribute value and all text, so a > ends a tag and a
// < starts one. A run of whitespace between two block tags is the renderer's
// layout rather than text, and is copied through like a tag, so no mark ever
// lands between a list and its item.
func cut(h string) ([]piece, []string) {
	var pieces []piece
	var shape []string
	seg, depth := 0, 0
	for i := 0; i < len(h); {
		if h[i] == '<' {
			end := strings.IndexByte(h[i:], '>')
			if end < 0 {
				end = len(h) - i - 1
			}
			tag := h[i : i+end+1]
			p := piece{html: tag, tag: true, seg: seg, starts: -1, depth: depth}
			name, closing := tagName(tag)
			switch {
			case name == "input":
				p.cmp = "[ ]"
				if strings.Contains(tag, " checked") {
					p.cmp = "[x]"
				}
			case slices.Contains(blockTags, name):
				if closing {
					shape = append(shape, "/"+name)
				} else {
					shape = append(shape, name)
				}
				seg++
				depth = 0
				p.seg, p.starts = seg, seg
			case !strings.HasPrefix(tag, "<!"):
				p.inline = true
				depth++
			}
			pieces = append(pieces, p)
			i += end + 1
			continue
		}
		end := strings.IndexByte(h[i:], '<')
		if end < 0 {
			end = len(h) - i
		}
		run := h[i : i+end]
		i += end
		if strings.TrimSpace(run) == "" && betweenBlocks(h, i, pieces) {
			pieces = append(pieces, piece{html: run, tag: true, seg: seg, starts: -1, depth: depth})
			continue
		}
		for _, t := range textTokens(run) {
			pieces = append(pieces, piece{html: t, cmp: t, seg: seg, starts: -1, depth: depth})
		}
	}
	return pieces, shape
}

// betweenBlocks reports whether whitespace ending at i sits between two block
// tags: the last piece before it is one, and so is the tag after it.
func betweenBlocks(h string, i int, before []piece) bool {
	if len(before) == 0 || !isBlockTag(before[len(before)-1].html) {
		return false
	}
	end := strings.IndexByte(h[i:], '>')
	return i >= len(h) || end >= 0 && isBlockTag(h[i:i+end+1])
}

// isBlockTag reports whether a piece is a tag of a block element.
func isBlockTag(tag string) bool {
	name, _ := tagName(tag)
	return strings.HasPrefix(tag, "<") && slices.Contains(blockTags, name)
}

// tagName is a tag's element name, lowercased, and whether it closes one.
func tagName(tag string) (string, bool) {
	t := strings.TrimPrefix(tag, "<")
	closing := strings.HasPrefix(t, "/")
	t = strings.TrimPrefix(t, "/")
	end := strings.IndexFunc(t, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	})
	if end < 0 {
		end = len(t)
	}
	return strings.ToLower(t[:end]), closing
}

// textTokens cuts escaped text into the tokens a changed code line uses, with
// each character reference one token. Cut on word characters, &lt; would be
// three, and &lt; against &gt; would mark only the letters, leaving half an
// entity on each side of a marking element.
func textTokens(run string) []string {
	var out []string
	plain := 0
	for i := 0; i < len(run); i++ {
		if run[i] != '&' {
			continue
		}
		m := entity.FindString(run[i:])
		if m == "" {
			continue
		}
		out = append(out, diff.Words(run[plain:i])...)
		out = append(out, m)
		i += len(m) - 1
		plain = i + 1
	}
	return append(out, diff.Words(run[plain:])...)
}

// compared is the index of every piece the comparison reads.
func compared(pieces []piece) []int {
	var out []int
	for i, p := range pieces {
		if p.cmp != "" {
			out = append(out, i)
		}
	}
	return out
}

// cmps is what the comparison reads of each compared piece.
func cmps(pieces []piece, at []int) []string {
	out := make([]string, len(at))
	for i, pi := range at {
		out[i] = pieces[pi].cmp
	}
	return out
}
