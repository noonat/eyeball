package diff

// hunks groups a script's changes into the runs a reader sees, each with
// context around it.
//
// Two changes closer together than twice the context share a hunk, because
// splitting them would print the same lines twice and make a reader reconcile
// two headers over one edit.
func hunks(script []step, oldLines, newLines []string, oldEnds, newEnds bool, context int) []Hunk {
	groups := group(script, context)
	if len(groups) == 0 {
		return nil
	}
	out := make([]Hunk, 0, len(groups))
	for _, g := range groups {
		out = append(out, build(script[g.from:g.to], oldLines, newLines, oldEnds, newEnds))
	}
	return out
}

// span is a half-open range of script positions.
type span struct {
	from, to int
}

// group finds the stretches of a script worth showing, merging any two whose
// context would overlap.
func group(script []step, context int) []span {
	var groups []span
	for i := 0; i < len(script); i++ {
		if script[i].op == OpContext {
			continue
		}
		from := i - context
		if from < 0 {
			from = 0
		}
		to := i + context + 1
		if to > len(script) {
			to = len(script)
		}
		if n := len(groups); n > 0 && from <= groups[n-1].to {
			groups[n-1].to = to
			continue
		}
		groups = append(groups, span{from: from, to: to})
	}
	return groups
}

// build turns one stretch of script into a hunk, numbering its lines and
// marking the last line of a file that does not end in a newline.
func build(steps []step, oldLines, newLines []string, oldEnds, newEnds bool) Hunk {
	h := Hunk{Lines: make([]Line, 0, len(steps))}
	for _, s := range steps {
		line := Line{Op: s.op}
		switch s.op {
		case OpAdd:
			line.New = s.new + 1
			line.Text = newLines[s.new]
			line.NoNewline = !newEnds && s.new == len(newLines)-1
		case OpRemove:
			line.Old = s.old + 1
			line.Text = oldLines[s.old]
			line.NoNewline = !oldEnds && s.old == len(oldLines)-1
		default:
			line.Old = s.old + 1
			line.New = s.new + 1
			line.Text = oldLines[s.old]
			// A context line matched on both sides, and the ending is part of
			// what a line matches on, so asking either side gives the same
			// answer. Reading the new side is not a choice between them.
			line.NoNewline = !newEnds && s.new == len(newLines)-1
		}
		h.Lines = append(h.Lines, line)
		count(&h, line)
	}
	return h
}

// count extends a hunk's ranges to cover one more line.
func count(h *Hunk, line Line) {
	if line.Old != 0 {
		if h.OldLines == 0 {
			h.OldStart = line.Old
		}
		h.OldLines++
	}
	if line.New != 0 {
		if h.NewLines == 0 {
			h.NewStart = line.New
		}
		h.NewLines++
	}
}
