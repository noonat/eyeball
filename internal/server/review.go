package server

import (
	"context"
	"fmt"
	"html/template"
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
	// ProjectHref is the project's view, which the brief's project links to.
	ProjectHref string
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
	// Fresh is the one file shown, rendered on the page rather than in a
	// card, where the round shows one markdown file whose old side has no
	// blocks. It is nil otherwise.
	Fresh *fileView
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
	// AllFiles is the tree's root at this round, keeping the page's
	// comparison.
	AllFiles string
	// Acting is what the reviewer can do on the page, and nil on a round
	// nothing more can be said on.
	Acting *actingView
	// Unmeasurable says why nothing could be compared, where git can no
	// longer read the round's base, and is empty otherwise. The files are then
	// shown as the round captured them.
	Unmeasurable string
}

// actingView is what the reviewer can do on the current round: comment and
// decide while it is open, or add a late comment once it is decided.
type actingView struct {
	// Review and Round are what every write names.
	Review int64
	Round  int
	// State is "open" on an undecided round and "late" on a decided one. The
	// client reads it to know whether a tap opens a composer.
	State string
	// Against is the page's comparison, which a removed row's quote needs.
	Against string
	// Composer is the review's own box under the agent's note.
	Composer composerView
	// List is the review-level comments, where that box's answers go.
	List listView
	// Bar is the decision bar, and nil once the round is decided.
	Bar *barView
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
	// IsDocument says the file is shown as a document rather than as rows.
	IsDocument bool
	// Document is a markdown file rendered with what moved marked.
	Document template.HTML
	// Whole is the file's own page, where it is shown whole, keeping the
	// page's comparison.
	Whole string
	// RawDiff is a document's Source view on its own page, keeping the page's
	// comparison, and empty for a file shown as rows.
	RawDiff string
	// Comments are the round's comments on it.
	Comments []commentView
	// List holds them as cards on the current round, where a composer's
	// answer replaces it, and is nil elsewhere.
	List *listView
	// Slot is the element id a tapped line's composer is swapped into, and
	// empty where no composer opens.
	Slot string
	// Unmeasured says the file has no counts, because nothing could be
	// compared, and Deleted that the round captured it as deleted.
	Unmeasured, Deleted bool
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
		Title:       review.Title,
		Project:     project.Name,
		ProjectHref: projectHref(project.ID),
		Agent:       review.Agent,
		Round:       round.Number,
		Abandoned:   review.State == store.StateAbandoned,
		Note:        paragraphs(round.Note),
		Waiting:     render.Age(now, round.Created),
	}

	decision, err := s.db.s.Decision(ctx, round.ID)
	decided := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return reviewPage{}, err
	}
	// A base git can no longer read, a lost repository or a missing root,
	// leaves nothing to compare against. The round's capture still holds every
	// file the agent changed, so the page shows those as captured and the
	// review can still be read and decided.
	st, err := capture.Inspect(ctx, project.Root, round.BaseCommit, project.InsideCheckout)
	if err != nil {
		return reviewPage{}, err
	}
	var moved []capture.Moved
	var before, after *capture.Reader
	if st.Measurable() {
		if moved, before, after, err = s.compare(ctx, project.Root, review, round, wholeReview); err != nil {
			return reviewPage{}, err
		}
	} else if err := s.captured(ctx, &view, st, review, round); err != nil {
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
	view.AllFiles = withQuery(roundHref(review.ID, round.Number, current)+"/files/", against)
	if current {
		if view.Earlier, err = s.earlierRounds(ctx, review, now); err != nil {
			return reviewPage{}, err
		}
	}

	files, added, removed := 0, 0, 0
	byPath := map[string]int{}
	// lastOld is the old side of the last document shown, which decides the
	// fresh layout when it is the only file.
	var lastOld []byte
	for _, m := range moved {
		if !store.Covers(review.Paths, m.Path) {
			continue
		}
		files++
		added += m.Result.Added
		removed += m.Result.Removed
		byPath[m.Path] = len(view.Shown)
		at := fileAt{review: review, round: round, current: current, wholeReview: wholeReview, path: m.Path}
		file := fileView{
			Path:    m.Path,
			Added:   thousands(m.Result.Added),
			Removed: thousands(m.Result.Removed),
			Whole:   at.href(m.Path),
		}
		// The result's flags cover the line limit as well as the byte limit,
		// and a document past either shows the too-large line. Each block
		// starts a line of its own, so that is also what keeps the block
		// comparison under its own limit, past which a document would show
		// with nothing marked.
		if !at.markdown() || m.Result.Binary || m.Result.TooLarge {
			file.Diff = render.Diff(m.Path, m.Result)
			view.Shown = append(view.Shown, file)
			continue
		}
		old, _, err := before.Side(ctx, m.Path)
		if err != nil {
			return reviewPage{}, err
		}
		content, _, err := after.Side(ctx, m.Path)
		if err != nil {
			return reviewPage{}, err
		}
		// Two documents on one page could repeat a heading id, so a card
		// writes none, and a fragment link goes to the document's own page.
		doc := at.page()
		doc.HeadingIDs = false
		file.IsDocument = true
		file.Document = render.Document(old, content, doc)
		file.RawDiff = at.viewHref("source")
		lastOld = old
		view.Shown = append(view.Shown, file)
	}
	expand := files <= maxOpenFiles && added+removed <= maxOpenLines
	for i := range view.Shown {
		view.Shown[i].Open = expand
	}
	if view.Unmeasurable == "" {
		view.Files = count(files, "file", "files")
		view.Added, view.Removed = thousands(added), thousands(removed)
	}
	// One new document on its own reads as a document, with nothing marked
	// and no card around it: marking every passage marks none.
	if len(view.Shown) == 1 && view.Shown[0].IsDocument && !render.HasBlocks(lastOld) {
		view.Fresh = &view.Shown[0]
	}

	open := current && !decided && review.State == store.StateOpen
	var decisionAt *store.Decision
	if decided {
		decisionAt = &decision
	}
	if current && review.State != store.StateAbandoned {
		view.Acting = s.acting(review, round, open, against.Get("against"))
		for i := range view.Shown {
			p := view.Shown[i].Path
			view.Shown[i].List = &listView{ID: listID(store.LevelFile, p)}
			if open {
				view.Shown[i].Slot = slotID(p)
			}
		}
	}

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
		// The fresh layout has no card to hold a file's comments, so they
		// join the review's own.
		if i, ok := byPath[c.Path]; ok && current && view.Fresh == nil {
			view.Shown[i].Comments = append(view.Shown[i].Comments, card)
			if l := view.Shown[i].List; l != nil {
				l.Cards = append(l.Cards, cardOf(review.ID, c, open, decisionAt))
			}
			continue
		}
		if view.Acting != nil && c.Level == store.LevelReview {
			view.Acting.List.Cards = append(view.Acting.List.Cards, cardOf(review.ID, c, open, decisionAt))
			continue
		}
		view.Comments = append(view.Comments, card)
	}

	if view.Acting != nil && view.Acting.Bar != nil {
		view.Acting.Bar.Tally = tallyView{Count: len(comments)}
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
//
// It returns the two readers as well, so a document reads its sides through
// the object ids the comparison already loaded in one batch.
func (s *Server) compare(ctx context.Context, root string, review store.Review, round store.Round, wholeReview bool) ([]capture.Moved, *capture.Reader, *capture.Reader, error) {
	current, err := s.roundFiles(ctx, round.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	var previous []store.File
	if round.Number > 1 && !wholeReview {
		before, err := s.db.s.Round(ctx, review.ID, round.Number-1)
		if err != nil {
			return nil, nil, nil, err
		}
		if previous, err = s.roundFiles(ctx, before.ID); err != nil {
			return nil, nil, nil, err
		}
	}
	before := capture.NewReader(root, round.BaseCommit, s.blobs, previous)
	now := capture.NewReader(root, round.BaseCommit, s.blobs, current)
	moved, err := capture.Compare(ctx, before, now)
	return moved, before, now, err
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

// acting is what the current round offers the reviewer.
func (s *Server) acting(review store.Review, round store.Round, open bool, against string) *actingView {
	a := &actingView{Review: review.ID, Round: round.Number, State: "late", Against: against}
	a.Composer, _ = composer(review.ID, round.Number, store.LevelReview, "", 0, 0, 0, "")
	a.List = listView{ID: listID(store.LevelReview, "")}
	if open {
		a.State = "open"
		a.Bar = &barView{Action: decisionHref(review.ID), Round: round.Number}
	} else {
		a.Composer.Hint, a.Composer.Button = "Sends immediately", "Send"
	}
	return a
}

// captured fills a page whose round cannot be compared with the files the
// review changes, each as the round captured it, with no counts.
func (s *Server) captured(ctx context.Context, view *reviewPage, st capture.State, review store.Review, round store.Round) error {
	stored, err := s.db.s.RoundFiles(ctx, round.ID)
	if err != nil {
		return err
	}
	files := make([]store.File, len(stored))
	for i, f := range stored {
		files[i] = f.File
	}
	reader := st.Reader(s.blobs, files)
	view.Unmeasurable = unmeasurable(st)
	shown := 0
	for _, f := range stored {
		if !f.InScope {
			continue
		}
		shown++
		at := fileAt{review: review, round: round, current: round.Number == review.LatestRound, path: f.Path}
		file := fileView{Path: f.Path, Whole: at.href(f.Path), Unmeasured: true, Open: true}
		if f.Digest == "" {
			file.Deleted = true
		} else {
			content, tooLarge, err := reader.Side(ctx, f.Path)
			if err != nil {
				return err
			}
			file.Diff = render.Plain(f.Path, content)
			file.Diff.TooLarge = file.Diff.TooLarge || tooLarge
		}
		view.Shown = append(view.Shown, file)
	}
	view.Files = count(shown, "file", "files")
	return nil
}
