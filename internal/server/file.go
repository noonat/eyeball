package server

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/kind"
	"github.com/noonat/eyeball/internal/render"
	"github.com/noonat/eyeball/internal/store"
)

// filePage is what the filepage template prints for one path at one round.
type filePage struct {
	// Dir is the path's directory with a trailing slash, empty at the root,
	// and Name its last segment.
	Dir, Name string
	// Notices are the banners above the file, in order.
	Notices []notice
	// Toolbar is the counts and the control choosing the view, and nil for a
	// file with neither.
	Toolbar *fileToolbar
	// Deleted is the sentence the empty box shows in place of the file, and
	// empty where the file is shown.
	Deleted string
	// ShowDocument says the file is shown as a document rather than as rows.
	ShowDocument bool
	// Document is a markdown file rendered, with heading ids.
	Document template.HTML
	// Diff is the file's rows, or the flag saying why it has none.
	Diff render.File
	// Rendered is the file shown as itself, a page in its frame, or the line
	// naming a file the surface does not show, and nil for rows or a
	// document.
	Rendered *rendering
	// Mode is the line saying the file's mode changed, or that an added file
	// is not a regular one, and empty otherwise.
	Mode string
}

// fileToolbar is the toolbar above a file that a comparison covers.
type fileToolbar struct {
	// Note says why there are no counts and no control, in their place:
	// nothing moved, or the change cannot be measured.
	Note string
	// Added and Removed are the lines each way, and Counted says what they
	// count.
	Added, Removed, Counted string
	// Marked and Plain are a code file's two views' addresses, and empty
	// where the control is not shown.
	Marked, Plain string
	// ShowPlain says the plain view is the one shown.
	ShowPlain bool
	// Rendered and Source are the two views' addresses of a file with a
	// rendering, markdown or a page, and empty where the control is not
	// shown. RenderedLabel names the rendering: Document, Page or Image.
	Rendered, RenderedLabel, Source string
	// ShowSource says the source view is the one shown.
	ShowSource bool
}

// notice is one banner.
type notice struct {
	// Warn says the banner warns rather than informs.
	Warn bool
	// Heading is the banner's bold opening, and Text the rest of it.
	Heading, Text string
}

// handleFilePage answers GET /reviews/{review}/files/{path...}: a path at the
// review's latest round.
func (s *Server) handleFilePage(w http.ResponseWriter, r *http.Request) error {
	review, ok, err := s.reviewFromPath(r)
	if err != nil || !ok {
		return s.missing(w, err)
	}
	return s.filePage(w, r, review, review.LatestRound)
}

// handleRoundFilePage answers GET /reviews/{review}/rounds/{round}/files/
// {path...}: a path at any round. The latest round answers here as well.
func (s *Server) handleRoundFilePage(w http.ResponseWriter, r *http.Request) error {
	number, err := strconv.Atoi(r.PathValue("round"))
	if err != nil {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	review, ok, err := s.reviewFromPath(r)
	if err != nil || !ok {
		return s.missing(w, err)
	}
	return s.filePage(w, r, review, number)
}

// reviewFromPath reads the review an address names, reporting false for one
// that does not exist.
func (s *Server) reviewFromPath(r *http.Request) (store.Review, bool, error) {
	id, err := strconv.ParseInt(r.PathValue("review"), 10, 64)
	if err != nil {
		return store.Review{}, false, nil
	}
	review, err := s.db.s.Review(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Review{}, false, nil
	}
	return review, err == nil, err
}

// missing answers with the 404 page where err is nil, and passes err on
// otherwise.
func (s *Server) missing(w http.ResponseWriter, err error) error {
	if err != nil {
		return err
	}
	s.errorPage(w, http.StatusNotFound)
	return nil
}

// filePage answers one path at one round of a review.
//
// The project's state is settled first, once, and everything after reads it:
// which reader is built, whether anything is compared, and which banners the
// page carries. A path is then located before anything is read, so a
// directory is never read as a file.
func (s *Server) filePage(w http.ResponseWriter, r *http.Request, review store.Review, number int) error {
	ctx := r.Context()
	round, err := s.db.s.Round(ctx, review.ID, number)
	if errors.Is(err, store.ErrNotFound) {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if err != nil {
		return err
	}
	project, err := s.db.s.Project(ctx, review.ProjectID)
	if err != nil {
		return err
	}
	root, err := s.copyRoot(ctx, round)
	if err != nil {
		return err
	}
	st, err := capture.Inspect(ctx, root, round.BaseCommit, project.InsideCheckout)
	if err != nil {
		return err
	}
	if st.FoundCheckout && !project.InsideCheckout {
		if err := s.db.MarkInsideCheckout(ctx, project.ID); err != nil {
			return err
		}
	}

	// The root is answered before anything reads a path, since CleanPath
	// refuses the empty one. Any other path is cleaned before it is located,
	// so a refusal there is the plain 404 page: no live read was asked for,
	// and a banner saying one was refused would be wrong.
	clean := ""
	if p := r.PathValue("path"); strings.Trim(p, "/") != "" {
		clean, err = capture.CleanPath(p)
		if errors.Is(err, capture.ErrNotFound) {
			s.errorPage(w, http.StatusNotFound)
			return nil
		}
		if err != nil {
			return err
		}
	}
	stored, err := s.db.s.RoundFiles(ctx, round.ID)
	if err != nil {
		return err
	}
	files := make([]store.File, len(stored))
	scope := make(map[string]bool, len(stored))
	views := make(map[string]string, len(stored))
	for i, f := range stored {
		files[i] = f.File
		scope[f.Path] = f.InScope
		views[f.Path] = f.View
	}
	at := fileAt{
		review:      review,
		round:       round,
		current:     round.Number == review.LatestRound,
		wholeReview: r.URL.Query().Get("against") == "base",
		view:        r.URL.Query().Get("view"),
		path:        clean,
		file:        kind.ByName(clean),
		stored:      views[clean],
		load:        r.URL.Query().Get("load") == "1",
	}
	if clean == "" {
		return s.treeView(ctx, w, at, project, st, files, scope)
	}
	now := st.Reader(s.blobs, files)
	pathType, source, err := now.Locate(ctx, clean)
	if errors.Is(err, capture.ErrNotFound) {
		s.notFoundPage(w, refusalNotice(st, err, s.tilde(st.Root)))
		return nil
	}
	if err != nil {
		return err
	}
	if pathType == capture.PathDir {
		return s.treeView(ctx, w, at, project, st, files, scope)
	}

	view := filePage{Name: path.Base(clean)}
	if dir := path.Dir(clean); dir != "." {
		view.Dir = dir + "/"
	}
	if n := stateNotice(st, s.tilde(st.Root)); n != nil {
		view.Notices = append(view.Notices, *n)
	}

	// A file the review changes goes back to its round, and any other file to
	// its directory in the tree. The address alone decides, since a page has
	// to render the same however it was reached.
	back, label := at.dirBack()
	switch source {
	case capture.SourceRound:
		inScope := scope[clean]
		if inScope {
			back, label = at.roundHref(), "Review"
			if !at.current {
				label = fmt.Sprintf("Round %d", round.Number)
			}
		}
		if !inScope {
			view.Notices = append(view.Notices, captureNotice(pathType == capture.PathDeleted, round.Number))
		}
		previous, err := s.previousFiles(ctx, at)
		if err != nil {
			return err
		}
		if at.file.Shown() || at.rendering() == kind.ViewImage {
			before := sideReader{reader: st.Reader(s.blobs, previous), number: at.previousNumber()}
			changed := st.Measurable() && capture.Differs(previous, files, at.path)
			if changed {
				if view.Mode, err = modeLine(ctx, before.reader, now, at.path); err != nil {
					return err
				}
			}
			if view.Rendered, err = s.renderShown(ctx, at, before, sideReader{reader: now, number: round.Number}, changed); err != nil {
				return err
			}
			if view.Rendered == nil && pathType == capture.PathDeleted {
				view.Deleted = fmt.Sprintf("The agent deleted this file as of round %d.", at.round.Number)
			}
			if at.file.Shown() {
				break
			}
		}
		rendered := view.Rendered
		if err := s.compareView(ctx, &view, at, st, now, previous, files, inScope, pathType == capture.PathDeleted); err != nil {
			return err
		}
		if rendered != nil {
			view.Rendered, view.Deleted = rendered, ""
		}
		if at.rendering() == kind.ViewPage && pathType != capture.PathDeleted {
			if view.Rendered, err = s.framePage(ctx, at, now); err != nil {
				return err
			}
			view.Notices = append(view.Notices, sandboxNotice)
		}
	case capture.SourceBase:
		view.Notices = append(view.Notices, baseNotice(store.Covers(review.Paths, clean)))
		if at.file.Shown() || at.rendering() == kind.ViewImage {
			if view.Rendered, err = s.renderShown(ctx, at, sideReader{}, sideReader{reader: now, number: round.Number}, false); err != nil {
				return err
			}
			break
		}
		if at.rendering() == kind.ViewPage {
			if view.Rendered, err = s.framePage(ctx, at, now); err != nil {
				return err
			}
			view.Notices = append(view.Notices, sandboxNotice)
			break
		}
		content, tooLarge, err := now.Side(ctx, clean)
		if err != nil {
			return err
		}
		view.showPlain(at, content, tooLarge)
	default:
		rc, err := capture.ReadLive(ctx, st, clean)
		if errors.Is(err, capture.ErrNotFound) {
			s.notFoundPage(w, refusalNotice(st, err, s.tilde(st.Root)))
			return nil
		}
		if err != nil {
			return err
		}
		content, err := readBounded(rc)
		if err != nil {
			return err
		}
		view.Notices = append(view.Notices, liveNotice(st))
		view.showPlain(at, content, false)
		switch {
		case at.file.Shown():
			// The raw route reads frozen rounds only, so a file read live
			// from the working copy is named rather than shown.
			view.Rendered = &rendering{Facts: &facts{Kind: at.file.Name, Size: bytesText(int64(len(content))), Hint: "Read from the working copy, which is not sent."}}
		case view.Diff.Binary:
			view.Rendered = &rendering{Facts: &facts{Kind: binaryName(at.file), Size: bytesText(int64(len(content)))}}
		}
	}
	view.viewControl(at)
	if view.Rendered == nil && view.Deleted == "" && (view.Diff.Binary || at.file.Kind == kind.Other) {
		if view.Rendered, err = nameBinary(ctx, at, now); err != nil {
			return err
		}
	}

	s.renderPage(w, http.StatusOK, "filepage", page{Title: view.Name, Back: back, BackLabel: label, Body: view})
	return nil
}

// fileAt is the address a file page answers, which every link it writes is
// built from.
type fileAt struct {
	review store.Review
	round  store.Round
	// current says the round is the review's latest, whose links use the
	// short form.
	current bool
	// wholeReview says the comparison is against the base rather than the
	// previous round, and every link carries it.
	wholeReview bool
	// view is the view asked for: marked or plain for a code file, document
	// or source for a markdown file, and empty for the default.
	view string
	path string
	// file is what the path is, by its name.
	file kind.File
	// stored is the view the agent chose for the path at this round, and
	// empty for the file's default.
	stored string
	// load says the reviewer asked for a file past the send limit anyway.
	load bool
}

// raw is the path's bytes as of a round, asking for them past the send limit
// where the reviewer did.
func (a fileAt) raw(number int) string {
	href := rawHref(a.review.ID, number, a.path)
	if a.load {
		href += "?load=1"
	}
	return href
}

// loadHref is this file's page in the view shown, with a file past the send
// limit sent anyway. A card has no view of its own, so its link opens the
// file's default.
func (a fileAt) loadHref() string {
	q := a.against()
	if a.view != "" {
		q.Set("view", a.view)
	}
	q.Set("load", "1")
	return withQuery(fileHref(a.review.ID, a.round.Number, a.current, a.path), q)
}

// chosen is the view the page opens in: the one asked for, then the one the
// agent chose, then the file's default.
func (a fileAt) chosen() string {
	if a.view != "" {
		return a.view
	}
	if a.stored != "" {
		return a.stored
	}
	return a.file.Default()
}

// markdown says the path is markdown, which has a document and a source.
func (a fileAt) markdown() bool {
	return a.file.Kind == kind.Markdown
}

// document says the path is shown as a document: a markdown file, in any
// view but Source. Marked and Plain are a code file's views, so either names
// the default.
func (a fileAt) document() bool {
	return a.markdown() && a.chosen() != kind.ViewSource
}

// rendering is the rendered view a page is shown in, Page or Image, and
// empty where it is shown as source.
func (a fileAt) rendering() string {
	if a.file.Kind == kind.Page && a.chosen() == a.file.Rendered() {
		return a.file.Rendered()
	}
	return ""
}

// previousNumber is the round the comparison reads its earlier side from,
// as the raw route numbers it: the previous round, or 0 for the base.
func (a fileAt) previousNumber() int {
	if a.round.Number == 1 || a.wholeReview {
		return 0
	}
	return a.round.Number - 1
}

// href is a project path's page at the address's round, keeping the
// comparison. It is how a document's links leave it.
func (a fileAt) href(p string) string {
	return withQuery(fileHref(a.review.ID, a.round.Number, a.current, p), a.against())
}

// page is what the address's document needs: its links resolved into the
// round, and the Source view an HTML block's placeholder links to. Heading ids
// are on, since a file page holds one document.
func (a fileAt) page() render.Page {
	return render.Page{Path: a.path, Href: a.href, SourceHref: a.viewHref("source"), HeadingIDs: true}
}

// against is the comparison as a query, empty for the default.
func (a fileAt) against() url.Values {
	q := url.Values{}
	if a.wholeReview {
		q.Set("against", "base")
	}
	return q
}

// roundHref is the round's page, keeping the comparison.
func (a fileAt) roundHref() string {
	return withQuery(roundHref(a.review.ID, a.round.Number, a.current), a.against())
}

// viewHref is this file's page in one view, keeping the comparison.
func (a fileAt) viewHref(view string) string {
	q := a.against()
	q.Set("view", view)
	return withQuery(fileHref(a.review.ID, a.round.Number, a.current, a.path), q)
}

// previousFiles is the capture the round is compared against: the previous
// round's, or none, which compares the round with its base.
func (s *Server) previousFiles(ctx context.Context, at fileAt) ([]store.File, error) {
	if at.round.Number == 1 || at.wholeReview {
		return nil, nil
	}
	before, err := s.db.s.Round(ctx, at.review.ID, at.round.Number-1)
	if err != nil {
		return nil, err
	}
	return s.roundFiles(ctx, before.ID)
}

// compareView fills in a captured file: its toolbar, and either its rows or
// the box saying it was deleted.
//
// Whether the file moved is decided from the two captures' digests, as the
// review page decides which files it lists, so the two cannot disagree about
// a file. The rows come from diff.Whole, which marks what the review page's
// hunks marked.
func (s *Server) compareView(ctx context.Context, view *filePage, at fileAt, st capture.State, now *capture.Reader, previous, current []store.File, inScope, deleted bool) error {
	deletedText := fmt.Sprintf("The agent deleted this file as of round %d.", at.round.Number)
	if !st.Measurable() {
		view.Toolbar = &fileToolbar{Note: unmeasurable(st)}
		if deleted {
			view.Deleted = deletedText
			return nil
		}
		content, tooLarge, err := now.Side(ctx, at.path)
		if err != nil {
			return err
		}
		view.showPlain(at, content, tooLarge)
		return nil
	}

	before := st.Reader(s.blobs, previous)
	if capture.Differs(previous, current, at.path) {
		line, err := modeLine(ctx, before, now, at.path)
		if err != nil {
			return err
		}
		view.Mode = line
	}
	old, tooLarge, err := before.Side(ctx, at.path)
	if err != nil {
		return err
	}
	content, currentTooLarge, err := now.Side(ctx, at.path)
	if err != nil {
		return err
	}
	if tooLarge || currentTooLarge {
		view.Diff = render.File{TooLarge: true}
		return nil
	}

	if !capture.Differs(previous, current, at.path) {
		if inScope && !deleted {
			view.Toolbar = &fileToolbar{Note: fmt.Sprintf("Not changed since round %d", at.round.Number-1)}
		}
		if deleted {
			view.Deleted = deletedText
			return nil
		}
		view.showPlain(at, content, false)
		return nil
	}

	// The line limit applies here as the byte limit does. A document's blocks
	// each start a line of their own, so one past the limit in lines is the
	// only way to reach the limit in blocks, and a document is never shown
	// whole with nothing marked for being too long to compare.
	result := diff.Whole(old, content)
	if result.Binary || result.TooLarge {
		view.Diff = render.Whole(at.path, result)
		return nil
	}
	counted := "in this round"
	if at.wholeReview && at.round.Number > 1 {
		counted = "in this review"
	}
	view.Toolbar = &fileToolbar{
		Added:   thousands(result.Added),
		Removed: thousands(result.Removed),
		Counted: counted,
	}
	if at.markdown() {
		// A document's change is a rule beside a passage, so there is no
		// tint to switch off and no Plain view. Source is the whole file
		// with the change marked, since the exact edit is the question once
		// a reviewer flips. A deleted document shows its removed blocks.
		if at.document() {
			view.ShowDocument = true
			view.Document = render.Document(old, content, at.page())
			return nil
		}
		view.Diff = render.Whole(at.path, result)
		return nil
	}
	plain := at.view == "plain"
	view.Toolbar.Marked, view.Toolbar.Plain, view.Toolbar.ShowPlain = at.viewHref("marked"), at.viewHref("plain"), plain
	switch {
	case !plain:
		view.Diff = render.Whole(at.path, result)
	case deleted:
		view.Deleted = deletedText
	default:
		view.Diff = render.Plain(at.path, content)
	}
	return nil
}

// showPlain fills in a file nothing marks: its rows, or a markdown file's
// document with nothing marked. A binary file, and one past the diff's limits
// or where reading stopped at the byte limit, shows the line saying so either
// way.
func (v *filePage) showPlain(at fileAt, content []byte, tooLarge bool) {
	if tooLarge {
		v.Diff = render.File{TooLarge: true}
		return
	}
	if r := diff.Plain(content); at.document() && !r.Binary && !r.TooLarge {
		v.ShowDocument = true
		v.Document = render.Document(nil, content, at.page())
		return
	}
	v.Diff = render.Plain(at.path, content)
}

// viewControl gives a file with a rendering its two views, beside whatever
// its toolbar already says: Document and Source for markdown, Page or Image
// and Source for a page. Every such file has both views, whether or not
// anything moved, so the control is not tied to a change as Marked and Plain
// are. A file shown as the binary or too-large line, or as the deleted box,
// has one view and no control.
func (v *filePage) viewControl(at fileAt) {
	rendered := at.file.Rendered()
	if rendered == "" || v.Deleted != "" || v.Diff.Binary || (v.Diff.TooLarge && v.Rendered == nil) {
		return
	}
	if v.Toolbar == nil {
		v.Toolbar = &fileToolbar{}
	}
	v.Toolbar.Marked, v.Toolbar.Plain = "", ""
	labels := map[string]string{kind.ViewDocument: "Document", kind.ViewPage: "Page", kind.ViewImage: "Image"}
	showSource := !at.document() && at.rendering() == ""
	v.Toolbar.Rendered, v.Toolbar.RenderedLabel, v.Toolbar.Source, v.Toolbar.ShowSource = at.viewHref(rendered), labels[rendered], at.viewHref(kind.ViewSource), showSource
}

// readBounded reads at most one byte past the diff's limit and closes rc. A
// file past the limit is refused by render.Plain on its length, so stopping
// there keeps a large file out of memory without changing the answer.
func readBounded(rc io.ReadCloser) ([]byte, error) {
	defer func() { _ = rc.Close() }()
	content, err := io.ReadAll(io.LimitReader(rc, diff.MaxBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, "read the working copy")
	}
	return content, nil
}

// captureNotice is the banner on a file the round captured outside the
// review's paths. The capture is exact and frozen, so it informs.
func captureNotice(deleted bool, round int) notice {
	text := fmt.Sprintf("Changed by the agent outside the review's paths, shown as it was when round %d was taken.", round)
	if deleted {
		text = fmt.Sprintf("Deleted by the agent outside the review's paths, as of round %d.", round)
	}
	return notice{Heading: "Not part of this review.", Text: text}
}

// baseNotice is the banner on a file read from the base commit. A review's
// base is fixed at its first round, so the banner names round 1 always.
func baseNotice(covered bool) notice {
	heading := "Not part of this review."
	if covered {
		heading = "Unchanged by this review."
	}
	return notice{Heading: heading, Text: "Unchanged since the commit round 1 was taken from."}
}

// liveNotice is the banner on a file read off disk, naming why nothing frozen
// holds it.
func liveNotice(st capture.State) notice {
	reason := "Nothing captured this file"
	switch {
	case st.BaseGone:
		reason = "The commit round 1 was taken from is gone"
	case st.Live == capture.LiveOpen:
		reason = "The project is outside git"
	}
	return notice{Warn: true, Heading: "Live, not from this round.", Text: reason + ", so it is shown as it is on disk now and may have moved."}
}

// stateNotice is the banner every file page of a project carries when the
// project's root or its repository is gone, and nil otherwise. Either way the
// round's capture is all that can be read.
func stateNotice(st capture.State, root string) *notice {
	switch st.Live {
	case capture.LiveRootMissing, capture.LiveLost:
		return refusalNotice(st, nil, root)
	}
	return nil
}

// refusalNotice is the banner on the 404 page for a path a live read might
// have answered, naming why it did not, and nil where a live read was allowed
// and the path is simply not there.
//
// The reason comes from the state and from the error's sentinel, never from
// its text, which carries git's message and the operating system's.
func refusalNotice(st capture.State, err error, root string) *notice {
	heading := "Only what this round captured can be shown."
	var text string
	switch {
	case errors.Is(err, capture.ErrRulesUnreadable), st.Live == capture.LiveRulesUnreadable:
		heading, text = "Nothing can be read live from disk.", "Git could not say which files it ignores."
	case st.Live == capture.LiveRootMissing:
		text = "The project's root, " + root + ", no longer exists."
	case st.Live == capture.LiveLost:
		text = "The project's repository is gone from its root."
	case st.Live == capture.LiveIgnored:
		text = "The checkout around the project ignores it."
	case st.Live == capture.LiveCheckoutGone:
		text = "The checkout the project sat inside has lost its repository."
	case st.Live.Refused():
		// A refusing value added to capture without a case here still gets
		// a banner, rather than a nil that treeNotice would dereference.
		text = "Nothing on disk is read for this project."
	default:
		return nil
	}
	return &notice{Warn: true, Heading: heading, Text: text}
}

// unmeasurable is the toolbar's sentence where a captured file cannot be
// compared, because git can no longer read the round's base.
func unmeasurable(st capture.State) string {
	switch st.Live {
	case capture.LiveRootMissing:
		return "The project's root is gone, so the change cannot be measured."
	case capture.LiveLost:
		return "The project's repository is gone, so the change cannot be measured."
	}
	return "The commit this review started from is gone, so the change cannot be measured."
}

// notFoundPage writes the 404 page, with a banner where one says why.
func (s *Server) notFoundPage(w http.ResponseWriter, n *notice) {
	p := page{Title: "Not found", Back: "/", BackLabel: "Queue", Body: message{Heading: "Nothing here", Text: "There is no page at this address.", Notice: n}}
	s.renderPage(w, http.StatusNotFound, "error", p)
}

// roundHref is a round's page: the review's own address for its latest round,
// and the round's for an earlier one.
func roundHref(reviewID int64, number int, current bool) string {
	if current {
		return fmt.Sprintf("/reviews/%d", reviewID)
	}
	return fmt.Sprintf("/reviews/%d/rounds/%d", reviewID, number)
}

// fileHref is a file's page at a round, in the same form roundHref chooses.
func fileHref(reviewID int64, number int, current bool, p string) string {
	return roundHref(reviewID, number, current) + "/files/" + escapePath(p)
}

// escapePath writes a project path for an address, escaping each segment.
//
// Written raw, a#b.md is the path a with a fragment, q?.md is the path q with
// a query, and 50%.md does not parse. html/template leaves # and ? alone in an
// href, so the escaping is done here. url.PathEscape escapes a slash too,
// which is why it is applied a segment at a time.
func escapePath(p string) string {
	segments := strings.Split(p, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

// withQuery appends q to an address, or nothing for an empty q.
func withQuery(href string, q url.Values) string {
	if len(q) == 0 {
		return href
	}
	return href + "?" + q.Encode()
}
