package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/render"
	"github.com/noonat/eyeball/internal/store"
)

// maxWorkingRows bounds the working copy's rows, each of which costs a git
// process for HEAD's side and another for the ignore check on the disk's. Two
// hundred is a guess written down so it can be corrected.
const maxWorkingRows = 200

// projectPage is what the projectpage template prints for one project.
type projectPage struct {
	// Name is the project's name, and Root its root written with ~.
	Name, Root string
	// Open counts the open reviews, and Waiting and Working are their rows,
	// each in the queue's order.
	Open             int
	Waiting, Working []queueRow
	// Copies are the project's working copies on disk, the main checkout
	// first. A project with one is drawn as before working copies existed,
	// with no name or hint on its section.
	Copies []copySection
	// Live says some working copy lists rows, so the page says once that what
	// it lists is live and read only.
	Live bool
	// Named says each section carries its working copy's name and hint: there
	// are several, or the one there is is not at the project's root.
	Named bool
}

// copySection is one working copy on the project view.
type copySection struct {
	// Name is the working copy's directory, and Hint says what it is and
	// where, as "Main checkout, ~/src/harbor". Both are empty on a project
	// with one working copy.
	Name, Hint string
	// Notices are the banners saying what the working copy cannot show and
	// why.
	Notices []notice
	// Count is the number of files the working copy lists, written out, and
	// empty where nothing is listed.
	Count string
	// Capped is the notice saying how many files the rows leave out, and nil
	// where every file has a row.
	Capped *notice
	// Rows are the working copy's files, up to the cap.
	Rows []workingRow
	// Empty is the box shown in place of the rows.
	Empty *emptyBox
}

// workingRow is one changed file in the working copy.
type workingRow struct {
	// Path is the file's path, and Href its page: the claiming review's file
	// page, or the project file page.
	Path, Href string
	// Claimed says an open review covers the file, so the row carries no
	// counts.
	Claimed bool
	// Added and Removed are the lines each way against HEAD, and empty for
	// none.
	Added, Removed string
}

// emptyBox is the box a section shows when it has no rows.
type emptyBox struct {
	// Headline is the box's bold line, and Text the sentence under it.
	Headline, Text string
}

// openReview is an open review with what claiming a file needs.
type openReview struct {
	row store.ReviewRow
	// waiting says the review's latest round has no verdict yet.
	waiting bool
	paths   []string
	latest  int
	// copyID is the working copy its latest round came from, the only one
	// where its paths are claimed.
	copyID int64
}

// handleProjectPage answers GET /projects/{project}: the project's open
// reviews, then each working copy on disk as it is now.
func (s *Server) handleProjectPage(w http.ResponseWriter, r *http.Request) error {
	project, ok, err := s.projectFromPath(r)
	if err != nil || !ok {
		return s.missing(w, err)
	}
	ctx := r.Context()
	open, err := s.openReviews(ctx, project.ID)
	if err != nil {
		return err
	}

	view := projectPage{Name: project.Name, Root: s.tilde(project.Root), Open: len(open)}
	now := s.now()
	for _, o := range open {
		if o.waiting {
			view.Waiting = append(view.Waiting, withoutProject(s.queueRow(o.row, render.Age(now, o.row.RequestedAt))))
			continue
		}
		view.Working = append(view.Working, withoutProject(s.queueRow(o.row, render.Age(now, o.row.DecidedAt))))
	}
	copies, err := s.copiesOnDisk(ctx, project)
	if err != nil {
		return err
	}
	for _, c := range copies {
		// A single working copy that is not at the project's root is a
		// worktree whose main checkout is gone, so it is named for what it is.
		named := len(copies) > 1 || c.Root != project.Root
		view.Named = view.Named || named
		section, err := s.copySection(ctx, project, c, open, named)
		if err != nil {
			return err
		}
		view.Live = view.Live || len(section.Rows) > 0
		view.Copies = append(view.Copies, section)
	}
	s.renderPage(w, http.StatusOK, "projectpage", page{Title: project.Name, Back: "/", BackLabel: "Queue", Body: view})
	return nil
}

// copiesOnDisk is the project's working copies that are still on disk, the
// main checkout first. Where none is, it is the working copy at the project's
// root alone, so the page says that root is gone rather than showing nothing.
func (s *Server) copiesOnDisk(ctx context.Context, project store.Project) ([]store.WorkingCopy, error) {
	all, err := s.db.s.WorkingCopies(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	var out []store.WorkingCopy
	var atRoot store.WorkingCopy
	for _, c := range all {
		if c.Root == project.Root {
			atRoot = c
		}
		if onDisk(c.Root) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		atRoot.Root, atRoot.ProjectID = project.Root, project.ID
		out = append(out, atRoot)
	}
	return out, nil
}

// onDisk reports whether a root is still there. A root that cannot be looked
// at for another reason counts as there, since a page saying why is more use
// than a project that silently disappears.
func onDisk(root string) bool {
	_, err := os.Stat(root)
	return !errors.Is(err, os.ErrNotExist)
}

// copySection reads one working copy and fills in its section, its files
// claimed only by the open reviews whose latest round came from it.
func (s *Server) copySection(ctx context.Context, project store.Project, c store.WorkingCopy, open []openReview, named bool) (copySection, error) {
	var section copySection
	if named {
		section.Name = path.Base(c.Root)
		kind := "Worktree"
		if c.Main {
			kind = "Main checkout"
		}
		section.Hint = kind + ", " + s.tilde(c.Root)
	}
	wc, err := s.workingCopy(ctx, project, c.Root)
	if err != nil {
		return copySection{}, err
	}
	var here []openReview
	for _, o := range open {
		if o.copyID == c.ID {
			here = append(here, o)
		}
	}
	st := wc.State
	if wc.Listed {
		err := s.workingRows(ctx, &section, project, c, wc, here)
		if !errors.Is(err, capture.ErrRulesUnreadable) {
			return section, err
		}
		// git listed the working copy and then failed to say what it
		// ignores. Showing rows it could not check is what the rules exist
		// to prevent.
		st.Live = capture.LiveRulesUnreadable
		section.Count, section.Capped, section.Rows = "", nil, nil
	}
	section.Notices, section.Empty = unlisted(st, wc.ListErr, s.tilde(c.Root))
	return section, nil
}

// withoutProject is a queue row with its project left out, since the page is
// the project.
func withoutProject(row queueRow) queueRow {
	row.Project = ""
	return row
}

// workingRows fills in the working copy's rows: each file up to the cap,
// claimed ones marked and linked to their review, and the rest bold with
// their counts against HEAD.
func (s *Server) workingRows(ctx context.Context, view *copySection, project store.Project, c store.WorkingCopy, wc *capture.WorkingCopy, open []openReview) error {
	entries := wc.Entries
	if len(entries) == 0 {
		text := "Everything is committed."
		switch {
		case len(open) == 1:
			text = "Everything the open review covers is committed."
		case len(open) > 1:
			text = "Everything the open reviews cover is committed."
		}
		view.Empty = &emptyBox{Headline: "Nothing changed on disk", Text: text}
		return nil
	}
	view.Count = count(len(entries), "file", "files")
	if len(entries) > maxWorkingRows {
		view.Capped = &notice{Text: fmt.Sprintf("The first %d of %s files are listed.", maxWorkingRows, thousands(len(entries)))}
		entries = entries[:maxWorkingRows]
	}

	var unclaimed []string
	for _, e := range entries {
		row := workingRow{Path: e.Path, Href: projectFileHref(project.ID, copyParam(project, c), e.Path)}
		if o := claimant(open, e.Path); o != nil {
			row.Claimed, row.Href = true, fileHref(o.row.ReviewID, o.latest, true, e.Path)
		} else {
			unclaimed = append(unclaimed, e.Path)
		}
		view.Rows = append(view.Rows, row)
	}
	moved, err := wc.Compare(ctx, unclaimed)
	if err != nil {
		return err
	}
	counts := make(map[string]diff.Result, len(moved))
	for _, m := range moved {
		counts[m.Path] = m.Result
	}
	// A claimed row was not compared, so it finds no counts here.
	for i, row := range view.Rows {
		c := counts[row.Path]
		if c.Added > 0 {
			view.Rows[i].Added = thousands(c.Added)
		}
		if c.Removed > 0 {
			view.Rows[i].Removed = thousands(c.Removed)
		}
	}
	return nil
}

// claimant is the first open review, in the page's order, whose paths cover
// p, and nil where none does. Its numbers belong to the round that captured
// it, so the row links there rather than measuring the disk.
func claimant(open []openReview, p string) *openReview {
	for i := range open {
		if store.Covers(open[i].paths, p) {
			return &open[i]
		}
	}
	return nil
}

// unlisted is the banners and the box for a working copy git did not list,
// each saying why. listErr is the working copy's reason, read before the
// state: a git project whose HEAD names no commit is LiveIgnoreRules, which
// would otherwise read as a marker project inside a checkout.
func unlisted(st capture.State, listErr error, root string) ([]notice, *emptyBox) {
	const cannot = "Cannot be listed"
	if errors.Is(listErr, capture.ErrBadBase) {
		return nil, &emptyBox{Headline: cannot, Text: "The repository's HEAD names no commit, so git has nothing to compare the disk with."}
	}
	gitOnly := &emptyBox{Headline: cannot, Text: "What changed on disk is a question only git answers. Reviews still work; this view does not."}
	switch st.Live {
	case capture.LiveRootMissing:
		return nil, &emptyBox{Headline: cannot, Text: "The project's root, " + root + ", no longer exists."}
	case capture.LiveLost:
		return nil, &emptyBox{Headline: cannot, Text: "The project's repository is gone: its root no longer holds the .git it had."}
	case capture.LiveOpen:
		return []notice{{Warn: true, Heading: "No git repository here.", Text: "A round has nothing to diff against, so a first request will show whole files."}}, gitOnly
	case capture.LiveIgnoreRules:
		// A marker project inside a checkout that does not ignore it. Its
		// live reads follow the checkout's rules, and git could list the
		// checkout's changes below the marker, but the project has no base
		// of its own to list them against.
		return []notice{{Warn: true, Heading: "No repository of its own.", Text: "A marker file names this project inside another checkout. A round has nothing to diff against, so a first request will show whole files."}}, gitOnly
	}
	reason := "Git could not say which files it ignores."
	switch st.Live {
	case capture.LiveIgnored:
		reason = "The checkout around the project ignores it."
	case capture.LiveCheckoutGone:
		reason = "The checkout the project sat inside has lost its repository."
	}
	return []notice{{Warn: true, Heading: "Live browsing is unavailable.", Text: reason}}, &emptyBox{Headline: cannot, Text: "Nothing on disk is read for this project. Reviews still work, and the files their rounds captured still open."}
}

// handleProjectFilePage answers GET /projects/{project}/files/{path...}: one
// file the working copy lists, live, against HEAD.
//
// Only a listed path answers, including one past the rows' cap, so the route
// never becomes a reader of any file under the root. An unchanged file has
// nothing this view adds anyway.
func (s *Server) handleProjectFilePage(w http.ResponseWriter, r *http.Request) error {
	project, ok, err := s.projectFromPath(r)
	if err != nil || !ok {
		return s.missing(w, err)
	}
	ctx := r.Context()
	clean, err := capture.CleanPath(r.PathValue("path"))
	if errors.Is(err, capture.ErrNotFound) {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	if err != nil {
		return err
	}
	c, ok, err := s.copyFromQuery(ctx, r, project)
	if err != nil || !ok {
		return s.missing(w, err)
	}
	wc, err := s.workingCopy(ctx, project, c.Root)
	if err != nil {
		return err
	}
	entry, held := wc.Holds(clean)
	if !held {
		s.errorPage(w, http.StatusNotFound)
		return nil
	}
	head, disk, tooLarge, err := wc.Sides(ctx, clean)
	if errors.Is(err, capture.ErrRulesUnreadable) {
		s.notFoundPage(w, refusalNotice(wc.State, err, s.tilde(c.Root)))
		return nil
	}
	if err != nil {
		return err
	}

	view := filePage{
		Name:    path.Base(clean),
		Notices: []notice{{Warn: true, Heading: "Live, not from any round.", Text: "The file as it is on disk now, compared with HEAD. It moves while an agent works."}},
	}
	if dir := path.Dir(clean); dir != "." {
		view.Dir = dir + "/"
	}
	view.showWorking(projectFileHref(project.ID, copyParam(project, c), clean), clean, entry.Deleted, head, disk, tooLarge, r.URL.Query().Get("view") == "plain")
	s.renderPage(w, http.StatusOK, "filepage", page{Title: view.Name, Back: projectHref(project.ID), BackLabel: project.Name, Body: view})
	return nil
}

// showWorking fills in a working-copy file: its counts on disk and the Marked
// and Plain control, then the whole file with the change marked, or the file
// as it is on disk.
//
// A markdown file gets Marked and Plain too, not Document and Source. The page
// answers what moved on disk, which is a question about the edit, and nothing
// here is read as prose for a verdict.
func (v *filePage) showWorking(href, p string, deleted bool, head, disk []byte, tooLarge, plain bool) {
	if tooLarge {
		v.Diff = render.File{TooLarge: true}
		return
	}
	result := diff.Whole(head, disk)
	if result.Binary || result.TooLarge {
		v.Diff = render.Whole(p, result)
		return
	}
	v.Toolbar = &fileToolbar{
		Added:     thousands(result.Added),
		Removed:   thousands(result.Removed),
		Counted:   "on disk",
		Marked:    withQuery(href, url.Values{"view": {"marked"}}),
		Plain:     withQuery(href, url.Values{"view": {"plain"}}),
		ShowPlain: plain,
	}
	switch {
	case !plain:
		v.Diff = render.Whole(p, result)
	case deleted:
		v.Deleted = "This file is gone from disk."
	default:
		v.Diff = render.Plain(p, disk)
	}
}

// projectFromPath reads the project an address names, reporting false for one
// that does not exist.
//
// It reads the project by its id through store.Project, which reads whether
// the project was seen inside a checkout. A listing's row leaves that false,
// and Inspect given false for a marker project whose outer .git has gone would
// read it as a project outside git and allow its live reads.
func (s *Server) projectFromPath(r *http.Request) (store.Project, bool, error) {
	id, err := strconv.ParseInt(r.PathValue("project"), 10, 64)
	if err != nil {
		return store.Project{}, false, nil
	}
	project, err := s.db.s.Project(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Project{}, false, nil
	}
	return project, err == nil, err
}

// workingCopy reads one of a project's working copies, and records a .git that
// Inspect found at or above a marker root, as the file routes do.
func (s *Server) workingCopy(ctx context.Context, project store.Project, root string) (*capture.WorkingCopy, error) {
	wc, err := capture.ReadWorkingCopy(ctx, root, project.InsideCheckout)
	if err != nil {
		return nil, err
	}
	if wc.State.FoundCheckout && !project.InsideCheckout {
		if err := s.db.MarkInsideCheckout(ctx, project.ID); err != nil {
			return nil, err
		}
	}
	return wc, nil
}

// openReviews lists a project's open reviews in the page's order: waiting
// before working, the longest wait first and then the latest verdict first,
// as the queue orders each group.
func (s *Server) openReviews(ctx context.Context, projectID int64) ([]openReview, error) {
	rows, err := s.db.ReviewsIn(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var waiting, working []store.ReviewRow
	for _, row := range rows {
		switch {
		case row.State != store.StateOpen:
		case row.Verdict == "":
			waiting = append(waiting, row)
		default:
			working = append(working, row)
		}
	}
	sort.SliceStable(waiting, func(i, j int) bool {
		a, b := waiting[i], waiting[j]
		if !a.RequestedAt.Equal(b.RequestedAt) {
			return a.RequestedAt.Before(b.RequestedAt)
		}
		return a.ReviewID < b.ReviewID
	})
	sort.SliceStable(working, func(i, j int) bool {
		a, b := working[i], working[j]
		if !a.DecidedAt.Equal(b.DecidedAt) {
			return a.DecidedAt.After(b.DecidedAt)
		}
		return a.ReviewID < b.ReviewID
	})
	out := make([]openReview, 0, len(waiting)+len(working))
	for i, row := range append(waiting, working...) {
		review, err := s.db.s.Review(ctx, row.ReviewID)
		if err != nil {
			return nil, err
		}
		latest, err := s.db.s.Round(ctx, row.ReviewID, review.LatestRound)
		if err != nil {
			return nil, err
		}
		out = append(out, openReview{row: row, waiting: i < len(waiting), paths: review.Paths, latest: review.LatestRound, copyID: latest.WorkingCopyID})
	}
	return out, nil
}

// projectHref is a project's view.
func projectHref(projectID int64) string {
	return "/projects/" + strconv.FormatInt(projectID, 10)
}

// projectFileHref is a file's page in one of a project's working copies. A
// copy of zero is the one at the project's root, which the address leaves out,
// so a project with one working copy keeps the addresses it had.
func projectFileHref(projectID, copyID int64, p string) string {
	href := projectHref(projectID) + "/files/" + escapePath(p)
	if copyID == 0 {
		return href
	}
	return withQuery(href, url.Values{"copy": {strconv.FormatInt(copyID, 10)}})
}

// copyParam is the copy a file link names: zero for the working copy at the
// project's root, and the copy's id for any other.
func copyParam(project store.Project, c store.WorkingCopy) int64 {
	if c.Root == project.Root {
		return 0
	}
	return c.ID
}

// copyFromQuery reads the working copy a file address names with copy=, and
// the one at the project's root where it names none. It reports false for a
// copy that does not exist or belongs to another project.
func (s *Server) copyFromQuery(ctx context.Context, r *http.Request, project store.Project) (store.WorkingCopy, bool, error) {
	raw := r.URL.Query().Get("copy")
	if raw == "" {
		return store.WorkingCopy{ProjectID: project.ID, Root: project.Root}, true, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return store.WorkingCopy{}, false, nil
	}
	c, err := s.db.s.WorkingCopyByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && c.ProjectID != project.ID) {
		return store.WorkingCopy{}, false, nil
	}
	return c, err == nil, err
}
