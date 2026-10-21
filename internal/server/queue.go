package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/noonat/eyeball/internal/render"
	"github.com/noonat/eyeball/internal/store"
)

// settledShown is how many approved reviews the queue keeps. The tail exists
// to find a verdict given an hour ago without a search, not to be a history.
const settledShown = 5

// queuePage is what the queue template prints.
type queuePage struct {
	// Waiting, Working and Settled are the three sections, in that order.
	Waiting, Working, Settled []queueRow
	// Stamp is the queue's stamp when the page was rendered, which the
	// client compares with what its stream sends.
	Stamp string
}

// queueRow is one review as a queue row shows it.
type queueRow struct {
	// ID is the review's id, which its link is built from.
	ID int64
	// Project and Agent say where the review came from. Project is empty on
	// the project's own page, and the row leaves it out.
	Project, Agent string
	// Title is what the row is called.
	Title string
	// Age is how long the row has been in its section: since the round was
	// asked for while it waits, and since the verdict once there is one.
	Age string
	// Round is the latest round's number.
	Round int
	// Files is the count of files that moved, written out, and Added and
	// Removed the lines each way. A waiting row is the only one that prints
	// them.
	Files, Added, Removed string
	// Gist is the first paragraph of the latest round's note, which a waiting
	// row quotes under its title. It is split as the review page splits the
	// letter, so a note reads the same in both places.
	Gist string
}

// handleQueue answers GET /, the page the surface opens on.
func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) error {
	q, err := s.db.s.Queue(r.Context(), settledShown)
	if err != nil {
		return err
	}
	now := s.now()
	body := queuePage{}
	if body.Stamp, err = s.queueStamp(r.Context()); err != nil {
		return err
	}
	for _, row := range q.Waiting {
		body.Waiting = append(body.Waiting, s.queueRow(row, render.Age(now, row.RequestedAt)))
	}
	for _, row := range q.Working {
		body.Working = append(body.Working, s.queueRow(row, render.Age(now, row.DecidedAt)))
	}
	for _, row := range q.Settled {
		body.Settled = append(body.Settled, s.queueRow(row, render.Age(now, row.DecidedAt)))
	}
	// The queue's own Waiting section is the count, read in the same snapshot
	// as the rows, so the eye cannot disagree with the list under it.
	s.renderPage(w, http.StatusOK, "queue", page{Title: "Queue", Eye: &eye{Count: len(body.Waiting)}, Body: body})
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
		Round:   row.Round,
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
