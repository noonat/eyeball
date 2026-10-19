package server

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/store"
)

// maxTreeRows bounds a tree page. One directory can hold far more than a phone
// can scroll, and a thousand is a guess written down so it can be corrected.
const maxTreeRows = 1000

// treePage is what the treepage template prints for one directory at one
// round.
type treePage struct {
	// Dir is the crumb's lead, the project's name and the directory's parent,
	// and Name the directory's own name, or the project's at the root.
	Dir, Name string
	// Notices are the banners above the listing, in order.
	Notices []notice
	// Rows are the directory's entries, with the subdirectories that hold a
	// file the review changes expanded in place.
	Rows []treeRow
	// Empty is the sentence the empty box shows where there are no rows.
	Empty string
}

// treeRow is one entry of a tree page.
type treeRow struct {
	// Name is the entry's name, and Href its page.
	Name, Href string
	// Dir says the entry is a directory.
	Dir bool
	// Indent says the entry sits inside an expanded subdirectory.
	Indent bool
	// InScope says the entry is a file the review changes, which is bold.
	InScope bool
	// Added and Removed are the review's lines each way, and empty for none.
	Added, Removed string
}

// treeEntry is a listed entry and the depth it is shown at.
type treeEntry struct {
	capture.DirEntry
	indent bool
}

// treeView answers a directory at one round: the listing, the subdirectories
// it expands, and the counts for its bold rows.
//
// Bold is the round files' scope flag, the rule the file page's banner uses,
// so the two cannot disagree. The counts are the whole review's, against the
// base, because the tree's notice says bold is what the review changes, and a
// tree counting the latest change would show nothing for a file the review
// changed in round 1 and left alone since.
func (s *Server) treeView(ctx context.Context, w http.ResponseWriter, at fileAt, project store.Project, st capture.State, files []store.File, scope map[string]bool) error {
	now := st.Reader(s.blobs, files)
	entries, err := listTree(ctx, now, at.path, scope)
	if errors.Is(err, capture.ErrRulesUnreadable) {
		// git failed to say what it ignores in a batch, though the project's
		// own check passed. Listing the disk unchecked is what the rules
		// exist to prevent, so the capture alone is listed, as a failed live
		// read refuses.
		st.Live = capture.LiveRulesUnreadable
		now = st.Reader(s.blobs, files)
		entries, err = listTree(ctx, now, at.path, scope)
	}
	if errors.Is(err, capture.ErrNotFound) {
		// Locate found a directory a moment ago, and the disk no longer has
		// it.
		s.notFoundPage(w, refusalNotice(st, err, s.tilde(st.Root)))
		return nil
	}
	if err != nil {
		return err
	}

	view := treePage{
		Name:    project.Name,
		Notices: []notice{treeNotice(st, now.Listing(), s.tilde(st.Root))},
		Empty:   fmt.Sprintf("Nothing is here as of round %d.", at.round.Number),
	}
	if at.path != "" {
		view.Dir = project.Name + " / "
		if parent := path.Dir(at.path); parent != "." {
			view.Dir += parent + "/"
		}
		view.Name = path.Base(at.path)
	}
	if len(entries) > maxTreeRows {
		view.Notices = append(view.Notices, notice{Text: fmt.Sprintf("The first %s of %s entries are listed.", thousands(maxTreeRows), thousands(len(entries)))})
		entries = entries[:maxTreeRows]
	}

	var bold []string
	for _, e := range entries {
		row := treeRow{Name: e.Name, Dir: e.Dir, Indent: e.indent, InScope: !e.Dir && scope[e.Path]}
		if e.Dir {
			row.Href = at.treeHref(e.Path)
		} else {
			row.Href = withQuery(fileHref(at.review.ID, at.round.Number, at.current, e.Path), at.against())
		}
		if row.InScope {
			bold = append(bold, e.Path)
		}
		view.Rows = append(view.Rows, row)
	}
	if len(bold) > 0 && st.Measurable() {
		moved, err := capture.ComparePaths(ctx, st.Reader(s.blobs, nil), now, bold)
		if err != nil {
			return err
		}
		counts := make(map[string]diff.Result, len(moved))
		for _, m := range moved {
			counts[m.Path] = m.Result
		}
		for i, e := range entries {
			if !view.Rows[i].InScope {
				continue
			}
			c := counts[e.Path]
			if c.Added > 0 {
				view.Rows[i].Added = thousands(c.Added)
			}
			if c.Removed > 0 {
				view.Rows[i].Removed = thousands(c.Removed)
			}
		}
	}

	back, label := at.roundHref(), "Review"
	switch {
	case at.path != "":
		back, label = at.dirBack()
	case !at.current:
		label = fmt.Sprintf("Round %d", at.round.Number)
	}
	s.renderPage(w, http.StatusOK, "treepage", page{Title: "Files", Back: back, BackLabel: label, Body: view})
	return nil
}

// listTree lists a directory as the tree shows it: directories first, a
// deletion outside the review's paths dropped, and each subdirectory holding a
// file the review changes followed by its own entries, indented.
//
// The subdirectories are listed together, in one call, since they are
// siblings. Deeper ones inside them are rows and are not expanded, because the
// tree has one indent.
func listTree(ctx context.Context, now *capture.Reader, dir string, scope map[string]bool) ([]treeEntry, error) {
	listed, err := now.List(ctx, dir)
	if err != nil {
		return nil, err
	}
	shown := treeOrder(listed, scope)
	var expand []string
	for _, e := range shown {
		if e.Dir && holdsInScope(scope, e.Path) {
			expand = append(expand, e.Name)
		}
	}
	var below map[string][]capture.DirEntry
	if len(expand) > 0 {
		if below, err = now.ListSubdirs(ctx, dir, expand); err != nil {
			return nil, err
		}
	}
	out := make([]treeEntry, 0, len(shown))
	for _, e := range shown {
		out = append(out, treeEntry{DirEntry: e})
		for _, child := range treeOrder(below[e.Name], scope) {
			out = append(out, treeEntry{DirEntry: child, indent: true})
		}
	}
	return out, nil
}

// treeOrder is a listing as the tree shows it: directories first, then files,
// each in the byte order List gives, with a deletion outside the review's
// paths dropped, since that file is gone as of the round.
func treeOrder(listed []capture.DirEntry, scope map[string]bool) []capture.DirEntry {
	out := make([]capture.DirEntry, 0, len(listed))
	for _, e := range listed {
		if e.Deleted && !scope[e.Path] {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Dir && !out[j].Dir })
	return out
}

// holdsInScope reports whether a directory holds, at any depth, a file the
// review changes, a deletion included.
func holdsInScope(scope map[string]bool, dir string) bool {
	for p, in := range scope {
		if in && strings.HasPrefix(p, dir+"/") {
			return true
		}
	}
	return false
}

// treeNotice is the banner above a listing, saying where its entries come
// from. Every one ends by saying what bold means, as the tearout's does.
func treeNotice(st capture.State, listing capture.Listing, root string) notice {
	const bold = " Files this review changes are in bold."
	frozen := notice{Text: "The project as the agent had it." + bold}
	switch {
	case listing == capture.ListingFrozen:
		return frozen
	case listing == capture.ListingDisk:
		return notice{Warn: true, Heading: "Live, not from this round.", Text: "Files nothing captured are listed as they are on disk now, and may have moved." + bold}
	case st.Live.Refused():
		n := *refusalNotice(st, nil, root)
		n.Text += bold
		return n
	case st.BaseGone:
		return notice{Warn: true, Heading: "Only the files this round captured are listed.", Text: "The commit round 1 was taken from is gone, so any other file opens only by its address, as it is on disk now." + bold}
	}
	// A git project with no commit yet captured every file against the empty
	// tree, so its capture alone is the project as the agent had it.
	return frozen
}

// treeHref is a directory's page at the address's round, keeping the
// comparison. The empty path is the root.
func (a fileAt) treeHref(dir string) string {
	href := roundHref(a.review.ID, a.round.Number, a.current) + "/files/"
	if dir != "" {
		href += escapePath(dir) + "/"
	}
	return withQuery(href, a.against())
}

// dirBack is where the topbar goes back to from the address's path: the
// directory holding it, labeled with its name, or "Files" at the root.
func (a fileAt) dirBack() (string, string) {
	parent := path.Dir(a.path)
	if parent == "." {
		return a.treeHref(""), "Files"
	}
	return a.treeHref(parent), path.Base(parent)
}
