package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/render"
	"github.com/noonat/eyeball/internal/store"
)

// A round opens with every file expanded up to these sizes, and as a list of
// collapsed files past either. Ten files is product.md's number. Three hundred
// lines is this spec's reading of its "a few hundred", and both are an open
// question in product.md.
const (
	maxOpenFiles = 10
	maxOpenLines = 300
)

// reviewPage is what the review template prints for one round.
type reviewPage struct {
	// Title, Project and Agent say what the review is and where it came from.
	Title, Project, Agent string
	// Round is the round's number.
	Round int
	// Abandoned says the agent withdrew the review, which no round's verdict
	// records.
	Abandoned bool
	// Waiting is how long the round has been asked for, which the page shows
	// while it has no verdict.
	Waiting string
	// Decided is the verdict, and nil while the round waits.
	Decided *decidedView
	// Note is the agent's brief, a paragraph to an element.
	Note []string
	// Files, Added and Removed are the totals of the files shown.
	Files, Added, Removed string
	// Against is the control choosing what the change is measured against,
	// and nil on a first round, which has only its base.
	Against *againstControl
	// Shown is every in-scope file that moved, in path order.
	Shown []fileView
	// Comments are the comments that belong to no file shown: the review-level
	// ones, and any on a file this comparison does not show. On an earlier
	// round they are every comment, listed before the files as the tearout
	// lists them.
	Comments []commentView
	// Replies are the comments the agent has answered, each with its answer.
	Replies []commentView
	// Earlier lists the rounds before this one, on the current round only.
	Earlier []earlierRound
	// Closed says an earlier round is closed and which round is current, and
	// is empty on the current round.
	Closed string
}

// earlierRound is one row of the list of earlier rounds.
type earlierRound struct {
	// Href is the round's page.
	Href string
	// Label names the round and its verdict.
	Label string
	// Age is how long ago it was decided.
	Age string
}

// decidedView is a round's verdict as the page shows it.
type decidedView struct {
	// Approved says which verdict it was.
	Approved bool
	// Pill is the brief's state pill: the verdict and how long ago it went
	// out, or the verdict alone on a closed round.
	Pill string
	// Sentence says what went out with it and when.
	Sentence string
	// Note is the verdict note, a paragraph to an element, and empty for none.
	Note []string
}

// againstControl is the two-link control the tearout labels "Since round N"
// and "Whole review".
type againstControl struct {
	// Previous is the label of the previous-round link.
	Previous string
	// Base says the whole review is shown rather than the latest change.
	Base bool
}

// fileView is one file of a round.
type fileView struct {
	// Path is the file's path.
	Path string
	// Added and Removed are its lines each way.
	Added, Removed string
	// Open says the file starts expanded.
	Open bool
	// Diff is its rows, or the flag saying why it has none.
	Diff render.File
	// Whole is the file's own page, where it is shown whole, keeping the
	// page's comparison.
	Whole string
	// Comments are the round's comments on it.
	Comments []commentView
}

// commentView is one comment card.
type commentView struct {
	// Where is what the comment is attached to.
	Where string
	// Quoted is the text it points at, empty where it points at none.
	Quoted string
	// Body is what the reviewer wrote, or the agent's reply in a list of
	// replies.
	Body string
	// Late says it was written after the verdict.
	Late bool
}

// handleReviewPage answers GET /reviews/{review}: the review's latest round.
func (s *Server) handleReviewPage(w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(r.PathValue("review"), 10, 64)
	if err != nil {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	review, err := s.db.s.Review(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if err != nil {
		return err
	}
	view, err := s.roundView(r.Context(), review, review.LatestRound, r.URL.Query().Get("against") == "base")
	if err != nil {
		return err
	}
	s.renderPage(w, http.StatusOK, "review", page{Title: review.Title, Back: "/", BackLabel: "Queue", Body: view})
	return nil
}

// handleRoundPage answers GET /reviews/{review}/rounds/{round}: any round. The
// latest round is the review's page, under either address.
func (s *Server) handleRoundPage(w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(r.PathValue("review"), 10, 64)
	if err != nil {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	number, err := strconv.Atoi(r.PathValue("round"))
	if err != nil {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	review, err := s.db.s.Review(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if err != nil {
		return err
	}
	// A round the review does not have is the store's not-found, from the
	// round's own read.
	view, err := s.roundView(r.Context(), review, number, r.URL.Query().Get("against") == "base")
	if errors.Is(err, store.ErrNotFound) {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if err != nil {
		return err
	}
	p := page{Title: review.Title, Back: "/", BackLabel: "Queue", Body: view}
	if number < review.LatestRound {
		p = page{Title: fmt.Sprintf("Round %d", number), Back: "/reviews/" + strconv.FormatInt(review.ID, 10), BackLabel: "Review", Body: view}
	}
	s.renderPage(w, http.StatusOK, "review", p)
	return nil
}

// roundView reads one round of a review and everything its page prints.
func (s *Server) roundView(ctx context.Context, review store.Review, number int, wholeReview bool) (reviewPage, error) {
	project, err := s.db.s.Project(ctx, review.ProjectID)
	if err != nil {
		return reviewPage{}, err
	}
	round, err := s.db.s.Round(ctx, review.ID, number)
	if err != nil {
		return reviewPage{}, err
	}
	now := s.now()
	view := reviewPage{
		Title:     review.Title,
		Project:   project.Name,
		Agent:     review.Agent,
		Round:     round.Number,
		Abandoned: review.State == store.StateAbandoned,
		Note:      paragraphs(round.Note),
		Waiting:   render.Age(now, round.Created),
	}

	decision, err := s.db.s.Decision(ctx, round.ID)
	decided := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return reviewPage{}, err
	}
	moved, err := s.compare(ctx, project.Root, review, round, wholeReview)
	if err != nil {
		return reviewPage{}, err
	}
	comments, err := s.db.s.Comments(ctx, round.ID)
	if err != nil {
		return reviewPage{}, err
	}

	if round.Number > 1 {
		view.Against = &againstControl{Previous: fmt.Sprintf("Since round %d", round.Number-1), Base: wholeReview}
	}
	current := round.Number == review.LatestRound
	against := url.Values{}
	if wholeReview {
		against.Set("against", "base")
	}
	if current {
		if view.Earlier, err = s.earlierRounds(ctx, review, now); err != nil {
			return reviewPage{}, err
		}
	}

	files, added, removed := 0, 0, 0
	byPath := map[string]int{}
	for _, m := range moved {
		if !store.Covers(review.Paths, m.Path) {
			continue
		}
		files++
		added += m.Result.Added
		removed += m.Result.Removed
		byPath[m.Path] = len(view.Shown)
		view.Shown = append(view.Shown, fileView{
			Path:    m.Path,
			Added:   thousands(m.Result.Added),
			Removed: thousands(m.Result.Removed),
			Diff:    render.Diff(m.Path, m.Result),
			Whole:   withQuery(fileHref(review.ID, round.Number, current, m.Path), against),
		})
	}
	open := files <= maxOpenFiles && added+removed <= maxOpenLines
	for i := range view.Shown {
		view.Shown[i].Open = open
	}
	view.Files = count(files, "file", "files")
	view.Added, view.Removed = thousands(added), thousands(removed)

	batched := 0
	for _, c := range comments {
		late := decided && c.Created.After(decision.Decided)
		if !late {
			batched++
		}
		card := commentView{Where: where(c), Quoted: c.Quoted, Body: c.Body, Late: late}
		if c.Reply != "" {
			view.Replies = append(view.Replies, commentView{Where: card.Where, Body: c.Reply})
		}
		if i, ok := byPath[c.Path]; ok && current {
			view.Shown[i].Comments = append(view.Shown[i].Comments, card)
			continue
		}
		view.Comments = append(view.Comments, card)
	}

	if decided {
		went := "The verdict went out " + render.Ago(now, decision.Decided) + "."
		if batched > 0 {
			went = count(batched, "comment", "comments") + " and the verdict went out " + render.Ago(now, decision.Decided) + "."
		}
		view.Decided = &decidedView{
			Approved: decision.Verdict == store.VerdictApprove,
			Pill:     verdictText(decision.Verdict) + " " + render.AgeAgo(now, decision.Decided),
			Sentence: went,
			Note:     paragraphs(decision.Note),
		}
		if !current {
			view.Decided.Pill = verdictText(decision.Verdict)
			view.Closed = fmt.Sprintf("Closed %s. Round %d is the current one.", render.Ago(now, decision.Decided), review.LatestRound)
		}
	}
	return view, nil
}

// earlierRounds lists every round before a review's latest, newest first.
func (s *Server) earlierRounds(ctx context.Context, review store.Review, now time.Time) ([]earlierRound, error) {
	var out []earlierRound
	for n := review.LatestRound - 1; n >= 1; n-- {
		round, err := s.db.s.Round(ctx, review.ID, n)
		if err != nil {
			return nil, err
		}
		row := earlierRound{Href: fmt.Sprintf("/reviews/%d/rounds/%d", review.ID, n), Label: fmt.Sprintf("Round %d", n)}
		decision, err := s.db.s.Decision(ctx, round.ID)
		switch {
		case err == nil:
			row.Label += " \u00b7 " + strings.ToLower(verdictText(decision.Verdict))
			row.Age = render.AgeAgo(now, decision.Decided)
		case !errors.Is(err, store.ErrNotFound):
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// verdictText is a verdict as the page names it.
func verdictText(v store.Verdict) string {
	if v == store.VerdictApprove {
		return "Approved"
	}
	return "Changes requested"
}

// compare diffs a round against the previous one, or against its base for the
// whole review and on a first round. It is the comparison the round's stored
// size was summed from, so the page's totals and the queue's agree.
func (s *Server) compare(ctx context.Context, root string, review store.Review, round store.Round, wholeReview bool) ([]capture.Moved, error) {
	current, err := s.roundFiles(ctx, round.ID)
	if err != nil {
		return nil, err
	}
	var previous []store.File
	if round.Number > 1 && !wholeReview {
		before, err := s.db.s.Round(ctx, review.ID, round.Number-1)
		if err != nil {
			return nil, err
		}
		if previous, err = s.roundFiles(ctx, before.ID); err != nil {
			return nil, err
		}
	}
	before := capture.NewReader(root, round.BaseCommit, s.blobs, previous)
	now := capture.NewReader(root, round.BaseCommit, s.blobs, current)
	return capture.Compare(ctx, before, now)
}

// roundFiles is a round's capture as the reader takes it.
func (s *Server) roundFiles(ctx context.Context, roundID int64) ([]store.File, error) {
	stored, err := s.db.s.RoundFiles(ctx, roundID)
	if err != nil {
		return nil, err
	}
	out := make([]store.File, len(stored))
	for i, f := range stored {
		out[i] = f.File
	}
	return out, nil
}

// where is what a comment card names as its anchor.
func where(c store.Comment) string {
	switch {
	case c.Level == store.LevelReview:
		return "This review"
	case c.StartLine > 0 && c.EndLine > c.StartLine:
		return fmt.Sprintf("%s:%d–%d", c.Path, c.StartLine, c.EndLine)
	case c.StartLine > 0:
		return fmt.Sprintf("%s:%d", c.Path, c.StartLine)
	default:
		return c.Path
	}
}

// paragraphs splits text at blank lines, the way a note is written.
func paragraphs(text string) []string {
	var out []string
	for _, p := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// thousands writes n with a comma between each group of three digits, as the
// tearouts write a large count.
func thousands(n int) string {
	digits := strconv.Itoa(n)
	if n < 0 {
		return "-" + thousands(-n)
	}
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}
