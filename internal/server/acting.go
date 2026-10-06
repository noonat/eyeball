package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/store"
)

// The sentences a write answers with. Each is shown beside the control that
// made the request, so it says what happened in the reviewer's terms.
const (
	failedWrite     = "eyeball could not do that. The daemon's log says why."
	noRound         = "That round is not part of this review."
	noComment       = "That comment is not part of this review."
	notThisForm     = "This page could not have sent that. Reload it and try again."
	emptyComment    = "A comment needs some text."
	staleVerdict    = "This round was decided or withdrawn since the page loaded. The page reloads once nothing on it is unsaved."
	releasedComment = "This comment went out with the verdict, so it can no longer change."
)

// cardView is one comment's card, with Edit and Delete while it can change.
type cardView struct {
	// ID is the comment's id, and the card's element id is built from it.
	ID int64
	// Href is the comment's own address, which Edit and Delete act on.
	Href string
	// Where, Quoted and Body are what the card shows.
	Where, Quoted, Body string
	// Late says it was written after its round's verdict, or on a round that
	// can no longer be decided, so it went out on its own.
	Late bool
	// Editable says its round has not been released, so it can change.
	Editable bool
}

// tallyView is the bar's count of what the round holds.
type tallyView struct {
	// Count is the comments on the round.
	Count int
	// OOB marks the tally for htmx to swap by its id, beside a write's main
	// answer.
	OOB bool
}

// listView is the comments on one anchor: a file, or the review.
type listView struct {
	// ID is the list's element id, which a composer targets.
	ID string
	// Cards are the comments, in the order the store reads them.
	Cards []cardView
}

// barView is the decision bar on an open current round.
type barView struct {
	// Action is the address the verdict posts to.
	Action string
	// Round is the round the page showed, which the verdict names.
	Round int
	// Tally is the count shown above the buttons.
	Tally tallyView
	// Note is the verdict note as sent, kept when the bar is answered again.
	Note string
	// Refused says a change request with nothing in it was refused.
	Refused bool
}

// composerView is a composer anchored to a line, a passage, a file or the
// review.
type composerView struct {
	// Action is the address a comment posts to, and Target the list its answer
	// replaces.
	Action, Target string
	// Round is the round the page showed.
	Round int
	// Level and Path say what the comment attaches to.
	Level, Path string
	// Start and End are a line's or a passage's range, and OldLine a removed
	// row's number on the side it was removed from.
	Start, End, OldLine int
	// Against is the page's comparison, which says which side a removed row
	// was read from.
	Against string
	// Anchor is the line the composer names above its field.
	Anchor string
	// Chips switch the level without a request.
	Chips []chipView
	// Placeholder is the field's hint.
	Placeholder string
	// Hint is the line beside the button, and Button what the button reads.
	Hint, Button string
	// Slot is the element id of the slot a tapped line's composer fills, and
	// empty for the review's own box.
	Slot string
}

// chipView is one level a composer's text can be sent to.
type chipView struct {
	// Level is the level it sets, Label what it reads, and Target the list
	// a comment at that level goes in.
	Level, Label, Target string
	// Pressed marks the level the composer is on.
	Pressed bool
}

// handleAddComment answers POST /reviews/{review}/comments: one comment on the
// round the form names, whatever has happened to that round since.
//
// A round decided, withdrawn or followed by another takes the comment as late,
// and it goes out on its own. Refusing it would discard what somebody typed.
func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	round, ok, err := s.formRound(w, r, review)
	if !ok || err != nil {
		return err
	}
	if strings.TrimSpace(r.PostFormValue("body")) == "" {
		plain(w, http.StatusBadRequest, emptyComment)
		return nil
	}
	nc, problem, err := s.newComment(ctx, r, review, round)
	if err != nil {
		return err
	}
	if problem != "" {
		plain(w, http.StatusBadRequest, problem)
		return nil
	}
	stored, err := s.db.AddComment(ctx, nc)
	if err != nil {
		return err
	}
	return s.answerList(ctx, w, review, round, listID(stored.Level, stored.Path))
}

// newComment reads a comment form into what the store takes. The quote is read
// from the round's frozen content, never taken from the page: a second copy of
// text the server holds would have nothing checking that the two agree. A
// non-empty problem is the sentence a form that cannot be stored is refused
// with.
func (s *Server) newComment(ctx context.Context, r *http.Request, review store.Review, round store.Round) (store.NewComment, string, error) {
	form := r.PostForm
	level := store.Level(form.Get("level"))
	path := form.Get("path")
	start, startErr := optionalLine(form.Get("start"))
	end, endErr := optionalLine(form.Get("end"))
	old, oldErr := optionalLine(form.Get("oldline"))
	if startErr != nil || endErr != nil || oldErr != nil {
		return store.NewComment{}, notThisForm, nil
	}
	nc := store.NewComment{
		RoundID:    round.ID,
		Level:      level,
		Path:       path,
		Body:       form.Get("body"),
		Submission: form.Get("submission"),
	}
	switch level {
	case store.LevelReview:
		if path != "" || start != 0 || end != 0 || old != 0 {
			return nc, notThisForm, nil
		}
		return nc, "", nil
	case store.LevelFile:
		if path == "" || start != 0 || end != 0 {
			return nc, notThisForm, nil
		}
		if old == 0 {
			return nc, "", nil
		}
		quoted, problem, err := s.quote(ctx, review, round, path, old, old, true, form.Get("against") == "base")
		nc.Quoted = quoted
		return nc, problem, err
	case store.LevelLine, store.LevelPassage:
		if end == 0 {
			end = start
		}
		if path == "" || start == 0 || end < start || old != 0 || level == store.LevelLine && end != start {
			return nc, notThisForm, nil
		}
		quoted, problem, err := s.quote(ctx, review, round, path, start, end, false, false)
		nc.StartLine, nc.EndLine, nc.Quoted = start, end, quoted
		return nc, problem, err
	default:
		return nc, notThisForm, nil
	}
}

// quote reads lines first to last of path as of round, or, for a removed row,
// from the side of the comparison it was removed from: the previous round, or
// the base for the whole review.
func (s *Server) quote(ctx context.Context, review store.Review, round store.Round, path string, first, last int, removed, wholeReview bool) (string, string, error) {
	clean, err := capture.CleanPath(path)
	if errors.Is(err, capture.ErrNotFound) {
		return "", notThisForm, nil
	}
	if err != nil {
		return "", "", err
	}
	project, err := s.db.s.Project(ctx, review.ProjectID)
	if err != nil {
		return "", "", err
	}
	st, err := capture.Inspect(ctx, project.Root, round.BaseCommit, project.InsideCheckout)
	if err != nil {
		return "", "", err
	}
	var files []store.File
	if removed {
		files, err = s.previousFiles(ctx, fileAt{review: review, round: round, wholeReview: wholeReview})
	} else {
		files, err = s.roundFiles(ctx, round.ID)
	}
	if err != nil {
		return "", "", err
	}
	content, tooLarge, err := st.Reader(s.blobs, files).Side(ctx, clean)
	if err != nil || tooLarge || content == nil {
		// A path the round cannot read is the form's mistake, since every
		// line the page offers is one it read.
		return "", fmt.Sprintf("Round %d holds no readable copy of %s.", round.Number, clean), nil
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	if last > len(lines) {
		return "", fmt.Sprintf("%s has no line %d.", clean, last), nil
	}
	return strings.Join(lines[first-1:last], "\n"), "", nil
}

// handleEditComment answers PUT /reviews/{review}/comments/{comment}: a
// comment's new body, before the verdict releases it.
func (s *Server) handleEditComment(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	id, ok, err := s.reviewComment(w, r, review)
	if !ok || err != nil {
		return err
	}
	if strings.TrimSpace(r.PostFormValue("body")) == "" {
		plain(w, http.StatusBadRequest, emptyComment)
		return nil
	}
	err = s.db.EditComment(ctx, id, r.PostFormValue("body"))
	if errors.Is(err, store.ErrRoundClosed) {
		plain(w, http.StatusConflict, releasedComment)
		return nil
	}
	if err != nil {
		return err
	}
	card, err := s.card(ctx, review, id)
	if err != nil {
		return err
	}
	return s.fragments(w, http.StatusOK, fragment{name: "card", data: card})
}

// handleDeleteComment answers DELETE /reviews/{review}/comments/{comment}. A
// comment already gone is a success: the reviewer wanted it gone, and a delete
// sent twice must not report a failure.
func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	id, ok, err := s.reviewComment(w, r, review)
	if !ok || err != nil {
		return err
	}
	if id != 0 {
		err = s.db.DeleteComment(ctx, id)
		if errors.Is(err, store.ErrRoundClosed) {
			plain(w, http.StatusConflict, releasedComment)
			return nil
		}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	tally, err := s.tally(ctx, review)
	if err != nil {
		return err
	}
	return s.fragments(w, http.StatusOK, fragment{name: "tally", data: tally})
}

// handleDecide answers POST /reviews/{review}/decision: the verdict on the
// round the form names, which must still be the open review's latest and
// undecided. A recorded verdict reloads the page, since the decided page is a
// different page.
func (s *Server) handleDecide(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	round, ok, err := s.formRound(w, r, review)
	if !ok || err != nil {
		return err
	}
	verdict := store.Verdict(r.PostFormValue("verdict"))
	if verdict != store.VerdictApprove && verdict != store.VerdictChanges {
		plain(w, http.StatusBadRequest, notThisForm)
		return nil
	}
	note := r.PostFormValue("note")
	if review.State != store.StateOpen || round.Number != review.LatestRound {
		plain(w, http.StatusConflict, staleVerdict)
		return nil
	}
	err = s.db.Decide(ctx, round.ID, verdict, note)
	switch {
	case errors.Is(err, store.ErrEmptyChangeRequest):
		tally, err := s.tally(ctx, review)
		if err != nil {
			return err
		}
		bar := barView{Action: decisionHref(review.ID), Round: round.Number, Tally: tally, Note: note, Refused: true}
		return s.fragments(w, http.StatusUnprocessableEntity, fragment{name: "bar", data: bar})
	case errors.Is(err, store.ErrAlreadyDecided), errors.Is(err, store.ErrNotOpen):
		plain(w, http.StatusConflict, staleVerdict)
		return nil
	case err != nil:
		return err
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
	return nil
}

// handleComposer answers GET /reviews/{review}/composer: a composer anchored
// where the query says.
func (s *Server) handleComposer(w http.ResponseWriter, r *http.Request) error {
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	q := r.URL.Query()
	number, err := strconv.Atoi(q.Get("round"))
	if err != nil {
		plain(w, http.StatusNotFound, noRound)
		return nil
	}
	if _, err := s.db.s.Round(r.Context(), review.ID, number); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			plain(w, http.StatusNotFound, noRound)
			return nil
		}
		return err
	}
	start, startErr := optionalLine(q.Get("start"))
	end, endErr := optionalLine(q.Get("end"))
	old, oldErr := optionalLine(q.Get("oldline"))
	if startErr != nil || endErr != nil || oldErr != nil {
		plain(w, http.StatusBadRequest, notThisForm)
		return nil
	}
	c, ok := composer(review.ID, number, store.Level(q.Get("level")), q.Get("path"), start, end, old, q.Get("against"))
	if !ok {
		plain(w, http.StatusBadRequest, notThisForm)
		return nil
	}
	return s.fragments(w, http.StatusOK, fragment{name: "composer", data: c})
}

// composer builds a composer for one anchor, and reports a level and fields
// that do not go together.
func composer(reviewID int64, round int, level store.Level, path string, start, end, old int, against string) (composerView, bool) {
	c := composerView{
		Action:  commentsHref(reviewID),
		Round:   round,
		Level:   string(level),
		Path:    path,
		Start:   start,
		End:     end,
		OldLine: old,
		Against: against,
		Hint:    "Saved as you type",
		Button:  "Add",
	}
	if level != store.LevelReview {
		c.Slot = slotID(path)
	}
	fileList := "#" + listID(store.LevelFile, path)
	review := chipView{Level: string(store.LevelReview), Label: "Review", Target: "#" + listID(store.LevelReview, "")}
	file := chipView{Level: string(store.LevelFile), Label: "File", Target: fileList}
	switch level {
	case store.LevelReview:
		if path != "" || start != 0 || old != 0 {
			return c, false
		}
		c.Anchor, c.Placeholder = "This review", "Comment on the whole review…"
		review.Pressed = true
		c.Chips = nil
	case store.LevelFile:
		if path == "" || start != 0 {
			return c, false
		}
		c.Anchor, c.Placeholder = path, "Comment on this file…"
		if old != 0 {
			c.Anchor = fmt.Sprintf("%s, removed line %d", path, old)
		}
		file.Pressed = true
		c.Chips = []chipView{file, review}
	case store.LevelLine:
		if path == "" || start == 0 || old != 0 {
			return c, false
		}
		c.End = start
		c.Anchor, c.Placeholder = fmt.Sprintf("%s:%d", path, start), fmt.Sprintf("Comment on line %d…", start)
		c.Chips = []chipView{{Level: string(store.LevelLine), Label: fmt.Sprintf("Line %d", start), Target: fileList, Pressed: true}, file, review}
	case store.LevelPassage:
		if path == "" || start == 0 || end < start || old != 0 {
			return c, false
		}
		c.Anchor, c.Placeholder = fmt.Sprintf("%s:%d–%d", path, start, end), "Comment on this passage…"
		c.Chips = []chipView{{Level: string(store.LevelPassage), Label: "Passage", Target: fileList, Pressed: true}, file, review}
	default:
		return c, false
	}
	c.Target = "#" + listID(level, path)
	return c, true
}

// handleComment answers GET /reviews/{review}/comments/{comment}: its card,
// or with ?edit=1 its edit form while it can still change.
func (s *Server) handleComment(w http.ResponseWriter, r *http.Request) error {
	review, ok, err := s.writeReview(w, r)
	if !ok || err != nil {
		return err
	}
	id, ok, err := s.reviewComment(w, r, review)
	if !ok || err != nil {
		return err
	}
	if id == 0 {
		plain(w, http.StatusNotFound, noComment)
		return nil
	}
	card, err := s.card(r.Context(), review, id)
	if err != nil {
		return err
	}
	if r.URL.Query().Get("edit") == "1" && card.Editable {
		return s.fragments(w, http.StatusOK, fragment{name: "editform", data: card})
	}
	return s.fragments(w, http.StatusOK, fragment{name: "card", data: card})
}

// writeReview resolves the review a write's address names. A review that does
// not exist is a 404 with a sentence the page can show.
func (s *Server) writeReview(w http.ResponseWriter, r *http.Request) (store.Review, bool, error) {
	id, err := strconv.ParseInt(r.PathValue("review"), 10, 64)
	if err != nil {
		plain(w, http.StatusNotFound, "There is no such review.")
		return store.Review{}, false, nil
	}
	review, err := s.db.s.Review(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		plain(w, http.StatusNotFound, "There is no such review.")
		return store.Review{}, false, nil
	}
	if err != nil {
		return store.Review{}, false, err
	}
	return review, true, nil
}

// formRound resolves the round a write's form names within its review. The
// page sends the number of the round it showed, so a write is never applied to
// a round the reviewer did not see.
func (s *Server) formRound(w http.ResponseWriter, r *http.Request, review store.Review) (store.Round, bool, error) {
	number, err := strconv.Atoi(r.PostFormValue("round"))
	if err != nil {
		plain(w, http.StatusNotFound, noRound)
		return store.Round{}, false, nil
	}
	round, err := s.db.s.Round(r.Context(), review.ID, number)
	if errors.Is(err, store.ErrNotFound) {
		plain(w, http.StatusNotFound, noRound)
		return store.Round{}, false, nil
	}
	if err != nil {
		return store.Round{}, false, err
	}
	return round, true, nil
}

// reviewComment resolves the comment an address names. A comment of another
// review is a 404, so one review's page cannot touch another's comment. A
// comment that no longer exists answers id 0 and leaves the caller to decide.
func (s *Server) reviewComment(w http.ResponseWriter, r *http.Request, review store.Review) (int64, bool, error) {
	id, err := strconv.ParseInt(r.PathValue("comment"), 10, 64)
	if err != nil {
		plain(w, http.StatusNotFound, noComment)
		return 0, false, nil
	}
	owner, err := s.db.CommentReview(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		return 0, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	if owner != review.ID {
		plain(w, http.StatusNotFound, noComment)
		return 0, false, nil
	}
	return id, true, nil
}

// answerList answers a stored comment: the anchor's list, and the tally beside
// it while the round still has a bar to count for.
func (s *Server) answerList(ctx context.Context, w http.ResponseWriter, review store.Review, round store.Round, id string) error {
	comments, err := s.db.s.Comments(ctx, round.ID)
	if err != nil {
		return err
	}
	open, decided, err := s.roundOpen(ctx, review, round)
	if err != nil {
		return err
	}
	list := listView{ID: id}
	for _, c := range comments {
		if listID(c.Level, c.Path) == id {
			list.Cards = append(list.Cards, cardOf(review.ID, c, open, decided))
		}
	}
	parts := []fragment{{name: "commentlist", data: list}}
	if open {
		parts = append(parts, fragment{name: "tally", data: tallyView{Count: len(comments), OOB: true}})
	}
	return s.fragments(w, http.StatusOK, parts...)
}

// card reads one comment as its card.
func (s *Server) card(ctx context.Context, review store.Review, id int64) (cardView, error) {
	c, number, err := s.db.s.Comment(ctx, id)
	if err != nil {
		return cardView{}, err
	}
	round, err := s.db.s.Round(ctx, review.ID, number)
	if err != nil {
		return cardView{}, err
	}
	open, decided, err := s.roundOpen(ctx, review, round)
	if err != nil {
		return cardView{}, err
	}
	return cardOf(review.ID, c, open, decided), nil
}

// roundOpen says whether a round can still be decided, and when it was
// decided if it has been.
func (s *Server) roundOpen(ctx context.Context, review store.Review, round store.Round) (bool, *store.Decision, error) {
	d, err := s.db.Decision(ctx, round.ID)
	if errors.Is(err, store.ErrNotFound) {
		return review.State == store.StateOpen && round.Number == review.LatestRound, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return false, &d, nil
}

// cardOf is a comment's card on a round that is open or not. A comment on a
// round that can no longer be decided went out on its own: after the verdict,
// or on a round withdrawn before one.
func cardOf(reviewID int64, c store.Comment, open bool, decided *store.Decision) cardView {
	late := !open && (decided == nil || c.Created.After(decided.Decided))
	return cardView{
		ID:       c.ID,
		Href:     fmt.Sprintf("/reviews/%d/comments/%d", reviewID, c.ID),
		Where:    where(c),
		Quoted:   c.Quoted,
		Body:     c.Body,
		Late:     late,
		Editable: open,
	}
}

// tally counts the comments on the review's latest round.
func (s *Server) tally(ctx context.Context, review store.Review) (tallyView, error) {
	round, err := s.db.s.Round(ctx, review.ID, review.LatestRound)
	if err != nil {
		return tallyView{}, err
	}
	comments, err := s.db.s.Comments(ctx, round.ID)
	if err != nil {
		return tallyView{}, err
	}
	return tallyView{Count: len(comments), OOB: true}, nil
}

// listID is the element id of the comment list an anchor's comments go in: one
// per file, and one for the review. A path is hashed because it may hold
// characters an id selector cannot carry without escaping.
func listID(level store.Level, path string) string {
	if level == store.LevelReview || path == "" {
		return "comments-review"
	}
	sum := sha256.Sum256([]byte(path))
	return "comments-" + hex.EncodeToString(sum[:6])
}

// slotID is the element id of a file's composer slot, hashed as listID is.
func slotID(path string) string {
	return "slot-" + strings.TrimPrefix(listID(store.LevelFile, path), "comments-")
}

// commentsHref is where a comment posts, and decisionHref where a verdict
// does.
func commentsHref(reviewID int64) string {
	return fmt.Sprintf("/reviews/%d/comments", reviewID)
}

func decisionHref(reviewID int64) string {
	return fmt.Sprintf("/reviews/%d/decision", reviewID)
}

// optionalLine reads a line number that may be absent.
func optionalLine(v string) (int, error) {
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, errors.Newf("line %q is not a line number", v)
	}
	return n, nil
}

// fragment is one named template block and its data.
type fragment struct {
	name string
	data any
}

// fragments writes template blocks one after another, the first being the
// answer htmx swaps into its target and any after it swapped by their ids.
func (s *Server) fragments(w http.ResponseWriter, status int, parts ...fragment) error {
	// Cloned for the same reason renderPage clones: a template that has been
	// executed can no longer be cloned, and every page is rendered from a
	// clone of s.pages.
	t, err := s.pages.Clone()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, p := range parts {
		if err := t.ExecuteTemplate(&buf, p.name, p.data); err != nil {
			return errors.Wrapf(err, "render %s", p.name)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
	return nil
}

// plain answers with one sentence of plain text, which the page's failure
// handler shows beside the control that sent the request.
func plain(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(text))
}
