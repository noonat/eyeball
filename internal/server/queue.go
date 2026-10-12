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
	// Empty is the empty state, shown when nothing is waiting.
	Empty *emptyQueue
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

// emptyQueue is the queue with nothing waiting.
type emptyQueue struct {
	// Sentence counts the agents at work and names where.
	Sentence string
	// Projects is every project eyeball knows.
	Projects []projectRow
}

// projectRow is one project in the empty state.
type projectRow struct {
	// Href is the project's view.
	Href string
	// Name is the project's name, and Root its root written with ~.
	Name, Root string
	// Activity is the count of agents at work, or Idle.
	Activity string
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
	if len(body.Waiting) == 0 {
		projects, err := s.db.s.Projects(r.Context())
		if err != nil {
			return err
		}
		body.Empty = s.emptyQueue(projects)
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

// emptyQueue builds the empty state from every project.
func (s *Server) emptyQueue(projects []store.ProjectRow) *emptyQueue {
	out := &emptyQueue{}
	agents := 0
	var busy []string
	for _, p := range projects {
		activity := "Idle"
		if p.Agents > 0 {
			activity = count(p.Agents, "agent", "agents")
			agents += p.Agents
			busy = append(busy, p.Name)
		}
		out.Projects = append(out.Projects, projectRow{Href: projectHref(p.ID), Name: p.Name, Root: s.tilde(p.Root), Activity: activity})
	}
	switch agents {
	case 0:
		out.Sentence = "No agents are working."
	case 1:
		out.Sentence = "One agent is working in " + busy[0] + "."
	default:
		where := "in " + busy[0]
		if len(busy) > 1 {
			where = "across " + strings.Join(busy[:len(busy)-1], ", ") + " and " + busy[len(busy)-1]
		}
		out.Sentence = capitalize(number(agents)) + " agents are working " + where + "."
	}
	return out
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

// numbers are the counts a sentence spells out.
var numbers = []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}

// number spells a count under ten, and writes digits from ten up.
func number(n int) string {
	if n >= 0 && n < len(numbers) {
		return numbers[n]
	}
	return fmt.Sprint(n)
}

// capitalize upper-cases the first letter of a word that opens a sentence.
func capitalize(word string) string {
	if word == "" {
		return word
	}
	return strings.ToUpper(word[:1]) + word[1:]
}
