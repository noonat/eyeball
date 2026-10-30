package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/noonat/eyeball/internal/render"
	"github.com/noonat/eyeball/internal/store"
	"github.com/noonat/eyeball/internal/view"
)

// settledShown is how many approved reviews the queue keeps. The tail exists
// to find a verdict given an hour ago without a search, not to be a history.
const settledShown = 5

// queueRow is one review as a queue row shows it. The template and the JSON
// view share the type, so a row cannot read differently in the two.
type queueRow = view.Row

// queueView is the queue as the JSON view holds it, with a stamp computed from
// the rows it carries. The caller reads the queue once, so the stamp and the
// sections cannot come from two moments.
func (s *Server) queueView(q store.Queue) view.Queue {
	now := s.now()
	out := view.Queue{WaitingCount: int32(len(q.Waiting)), Stamp: queueStampOf(q)}
	for _, row := range q.Waiting {
		out.Waiting = append(out.Waiting, s.queueRow(row, render.Age(now, row.RequestedAt)))
	}
	for _, row := range q.Working {
		out.Working = append(out.Working, s.queueRow(row, render.Age(now, row.DecidedAt)))
	}
	for _, row := range q.Settled {
		out.Settled = append(out.Settled, s.queueRow(row, render.Age(now, row.DecidedAt)))
	}
	return out
}

// handleQueueView answers GET /api/, the queue as JSON.
func (s *Server) handleQueueView(w http.ResponseWriter, r *http.Request) error {
	q, err := s.db.s.Queue(r.Context(), settledShown)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.queueView(q))
	return nil
}

// queueRow is a store row as the queue prints it.
func (s *Server) queueRow(row store.ReviewRow, age string) queueRow {
	return queueRow{
		Gist:    gist(row.Note, row.Title),
		ID:      row.ReviewID,
		Project: row.ProjectName,
		Agent:   row.Agent,
		Title:   row.Title,
		Age:     age,
		Round:   int32(row.Round),
		Files:   count(row.Files, "file", "files"),
		Added:   thousands(row.Added),
		Removed: thousands(row.Removed),
	}
}

// gist is what a waiting row quotes from its note: the first paragraph, split
// as the review page splits the letter. A review opened without a title is
// titled with its note's first line, by defaultTitle, and quoting that line
// again under the title prints the same sentence twice, so such a note is
// quoted from the line after it.
func gist(note, title string) string {
	if title != "" && title == defaultTitle(note) {
		_, note, _ = strings.Cut(strings.TrimSpace(note), "\n")
	}
	if p := paragraphs(note); len(p) > 0 {
		return p[0]
	}
	return ""
}

// tilde writes a path under the home directory with ~ in its place.
func (s *Server) tilde(path string) string {
	if s.home == "" {
		return path
	}
	if path == s.home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, s.home+"/"); ok {
		return "~/" + rest
	}
	return path
}

// count writes n with the noun that agrees with it.
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
