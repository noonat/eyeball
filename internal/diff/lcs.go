package diff

import (
	"github.com/aymanbagabas/go-udiff/lcs"
)

// step is one entry of an edit script, holding the indices it consumes.
//
// A context step consumes one line from each side. A remove consumes one from
// the old side and an add one from the new, and the index it does not consume
// is left at zero and never read.
type step struct {
	op  Op
	old int
	new int
}

// compare produces the edit script over two line sequences.
//
// The search is x/tools' two-sided Myers, through go-udiff. It is bidirectional
// and bounded: where the edit distance grows too large it stops early and joins
// a forward and a backward partial subsequence, so a large refactor still reads
// as a diff. A one-sided search has to give up instead, and what it can offer
// then is the whole file as one removal run and one addition run, which for two
// files that share half their content is a worse reading than no diff at all.
//
// A line carries its own newline here, which is how the library tells a last
// line that ends in one from a last line that does not. Comparing the text
// alone reports a file that lost its final newline as unchanged, while the
// capture that recorded it has a different digest, so the round would show a
// file that moved no lines.
func compare(oldLines, newLines []string, oldEnds, newEnds bool) []step {
	a := withNewlines(oldLines, oldEnds)
	b := withNewlines(newLines, newEnds)
	// The conversion below walks these in order and assumes they do not
	// overlap, which is the same assumption the library makes of them in its
	// own unified output. Sorting here first was written and then removed: it
	// could not be made to fail, so it was a guard against nothing. What checks
	// the assumption is the property test, which replays every script and would
	// not reproduce the new side if the ranges arrived in another order.
	changes := lcs.DiffLines(a, b)

	var script []step
	oldAt, newAt := 0, 0
	for _, c := range changes {
		for oldAt < c.Start {
			script = append(script, step{op: OpContext, old: oldAt, new: newAt})
			oldAt++
			newAt++
		}
		for ; oldAt < c.End; oldAt++ {
			script = append(script, step{op: OpRemove, old: oldAt})
		}
		for ; newAt < c.ReplEnd; newAt++ {
			script = append(script, step{op: OpAdd, new: newAt})
		}
	}
	for oldAt < len(oldLines) {
		script = append(script, step{op: OpContext, old: oldAt, new: newAt})
		oldAt++
		newAt++
	}
	return script
}

// withNewlines puts each line back together with the newline that followed it,
// which is what the comparison is over.
func withNewlines(lines []string, ends bool) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		if !ends && i == len(lines)-1 {
			out[i] = line
			continue
		}
		out[i] = line + "\n"
	}
	return out
}
