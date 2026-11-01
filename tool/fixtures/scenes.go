package main

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
)

// scene is one page address in a world, drawn by the project view, the tree or
// the file page. The view and the template page's markup are written for it.
type scene struct {
	// file is the name the scene's files share.
	file string
	// world is the key of the world the scene is read from.
	world string
	// address is the page's path and query, built from the ids the world
	// holds.
	address func(ids map[string]int64) string
}

// worlds builds each world by key. The week is the harbor project and the
// others a review reads files from. The worktrees are harbor with a second
// working copy, once with the worktree's changes on disk and once committed.
var worlds = map[string]func(ctx context.Context) (*world, error){
	"week":                weekWorld,
	"worktrees":           func(ctx context.Context) (*world, error) { return worktreeWorld(ctx, false) },
	"worktrees-committed": func(ctx context.Context) (*world, error) { return worktreeWorld(ctx, true) },
}

// worldOrder is the order the worlds are built in, so a run is the same every
// time.
var worldOrder = []string{"week", "worktrees", "worktrees-committed"}

// The frames of docs/design/project.html and docs/design/widening.html that a
// working store can draw, each by the state it shows.
var scenes = []scene{
	{file: "project-populated", world: "week", address: projectAt("harbor")},
	{file: "project-clean", world: "week", address: projectAt("field-notes")},
	{file: "project-not-git", world: "week", address: projectAt("scratch")},
	{file: "project-worktrees", world: "worktrees", address: projectAt("harbor")},
	{file: "project-worktree-committed", world: "worktrees-committed", address: projectAt("harbor")},

	{file: "tree-root", world: "week", address: reviewAt("upload", "files/")},
	{file: "tree-subdir", world: "week", address: reviewAt("upload", "files/internal/")},

	{file: "file-whole", world: "week", address: reviewAt("upload", "files/internal/upload/worker.go")},
	{file: "file-plain", world: "week", address: reviewAt("upload", "files/internal/upload/worker.go?view=plain")},
	{file: "file-document", world: "week", address: reviewAt("product", "files/docs/product.md")},
	{file: "file-document-source", world: "week", address: reviewAt("product", "files/docs/product.md?view=source")},
	{file: "file-context", world: "week", address: reviewAt("upload", "files/internal/storage/store.go")},
	{file: "file-live", world: "week", address: reviewAt("upload", "files/internal/upload/queue.go")},
	{file: "file-image", world: "week", address: reviewAt("upload", "files/testdata/plate-04.png")},
	{file: "file-mode", world: "week", address: reviewAt("status", "files/bin/sync")},
	{file: "file-page", world: "week", address: reviewAt("status", "files/web/status.html")},
	{file: "file-image-changed", world: "week", address: reviewAt("status", "files/web/icon.png")},
	{file: "file-image-new", world: "week", address: reviewAt("status", "files/web/new.png")},
	{file: "file-image-deleted", world: "week", address: reviewAt("status", "files/web/old.png")},
	{file: "file-svg", world: "week", address: reviewAt("status", "files/web/badge.svg?view=image")},
	{file: "file-audio-changed", world: "week", address: reviewAt("status", "files/media/chime.wav")},
	{file: "file-video-new", world: "week", address: reviewAt("status", "files/media/clip.mp4")},
	{file: "file-pdf", world: "week", address: reviewAt("status", "files/docs/retry-policy.pdf")},
	{file: "file-too-large", world: "week", address: reviewAt("status", "files/media/capture-03.mov")},
	{file: "file-binary", world: "week", address: reviewAt("status", "files/media/fixture.zip")},
	{file: "file-worktree", world: "worktrees", address: worktreeFile},
}

// projectAt is a project's page.
func projectAt(name string) func(ids map[string]int64) string {
	return func(ids map[string]int64) string {
		return fmt.Sprintf("/projects/%d", ids[name])
	}
}

// reviewAt is a path under a review, at its latest round.
func reviewAt(review, rest string) func(ids map[string]int64) string {
	return func(ids map[string]int64) string {
		return fmt.Sprintf("/reviews/%d/%s", ids[review], rest)
	}
}

// worktreeFile is a file in the worktree's working copy, which only its copy
// parameter reaches.
func worktreeFile(ids map[string]int64) string {
	return fmt.Sprintf("/projects/%d/files/internal/upload/backoff.go?copy=%d", ids["harbor"], ids["harbor-retry copy"])
}

// weekWorld is harbor with a review waiting and one worked on, an eyeball
// document in its second round, a status page and the media it shows, a
// project whose changes are all committed and one outside git. The ages are
// the tearouts', so the screens read as the phone approved them.
func weekWorld(ctx context.Context) (*world, error) {
	w, err := newWorld(ctx)
	if err != nil {
		return nil, err
	}
	if err := seedWeek(w); err != nil {
		w.close()
		return nil, err
	}
	w.at = now
	return w, nil
}

// seedWeek writes every project and review of the week.
func seedWeek(w *world) error {
	harbor, err := w.repo("harbor", map[string]string{
		"docs/architecture.md":              archBefore,
		"internal/doc.go":                   "// Package internal holds harbor's own packages.\npackage internal\n",
		"internal/migrations/0001_init.sql": "CREATE TABLE jobs (id INTEGER PRIMARY KEY, path TEXT NOT NULL);\n",
		"internal/outbox/drain.go":          drainBefore,
		"internal/storage/store.go":         storeGo,
		"internal/upload/worker.go":         workerBefore,
		"testdata/plate-04.png":             pngImage(160, 107, color.RGBA{R: 0x38, G: 0x4a, B: 0x5c, A: 0xff}),
	})
	if err != nil {
		return err
	}
	// The drainer: changes requested twice, the latest 26 minutes ago.
	w.ago(50 * time.Minute)
	if err := w.edit(harbor, map[string]string{"internal/outbox/drain.go": drainRound1}); err != nil {
		return err
	}
	drain, err := w.open(harbor, "drain", "fix/outbox-drain", "Offline outbox drainer", "A first cut, to agree the shape.", []string{"internal/outbox"}, nil)
	if err != nil {
		return err
	}
	w.ago(40 * time.Minute)
	if err := w.changes(drain); err != nil {
		return err
	}
	w.ago(30 * time.Minute)
	if err := w.edit(harbor, map[string]string{"internal/outbox/drain.go": drainRound2, "internal/outbox/drain_test.go": drainTest}); err != nil {
		return err
	}
	if drain, err = w.next(harbor, "drain", drain, "Takes a context, and a test."); err != nil {
		return err
	}
	w.ago(26 * time.Minute)
	if err := w.changes(drain); err != nil {
		return err
	}

	// The upload worker: waiting an hour, a Go diff with word marks.
	w.ago(61 * time.Minute)
	if err := w.edit(harbor, map[string]string{"internal/upload/worker.go": workerAfter, "internal/upload/backoff.go": backoffGo}); err != nil {
		return err
	}
	note := "The worker retries three times with backoff, then gives up and records the failure on the job. Measured against the staging bucket: a cold connection takes 40s, so the first wait is 60s not 5s."
	if _, err := w.open(harbor, "upload", "feat/upload-retry", "Upload worker and its retry ladder", note, []string{"internal/upload"}, nil); err != nil {
		return err
	}
	// Written after every capture, so the working copy lists changes no round
	// holds: a file nothing captured, which a file page reads live, and a
	// document the review does not cover.
	if err := w.edit(harbor, map[string]string{"internal/upload/queue.go": queueGo, "docs/architecture.md": archAfter}); err != nil {
		return err
	}

	// The product document: round 1 asked for changes, round 2 waits.
	eyeball, err := w.repo("eyeball", map[string]string{"docs/product.md": productStart, "docs/voice.md": "# Voice\n\nPlain, simple English.\n"})
	if err != nil {
		return err
	}
	w.ago(8 * time.Hour)
	if err := w.edit(eyeball, map[string]string{"docs/product.md": productOne}); err != nil {
		return err
	}
	product, err := w.open(eyeball, "product", "spec/012-surface", "Product doc, second pass", "First pass at the widening section.", []string{"docs/product.md"}, nil)
	if err != nil {
		return err
	}
	w.ago(6 * time.Hour)
	if err := w.changes(product); err != nil {
		return err
	}
	w.ago(12 * time.Minute)
	if err := w.edit(eyeball, map[string]string{"docs/product.md": productTwo}); err != nil {
		return err
	}
	if _, err := w.next(eyeball, "product", product, "Both comments from round 1 are done."); err != nil {
		return err
	}

	// The status page: every kind a browser displays, and a mode change.
	w.ago(40 * time.Minute)
	status, err := w.repo("status-page", map[string]string{
		"bin/sync":        syncBefore,
		"docs/README.md":  "# Status page\n",
		"media/chime.wav": "RIFF....WAVEfmt  before",
		"web/icon.png":    pngImage(48, 48, color.RGBA{R: 0xb0, G: 0x4a, B: 0x3c, A: 0xff}),
		"web/old.png":     pngImage(32, 32, color.RGBA{R: 0x8a, G: 0xab, B: 0xff, A: 0xff}),
		"web/badge.svg":   svgBefore,
		"web/status.css":  statusCSS,
		"web/status.html": statusPage("Uploads"),
	})
	if err != nil {
		return err
	}
	if err := w.edit(status, map[string]string{
		"bin/sync":              syncAfter,
		"media/chime.wav":       "RIFF....WAVEfmt  after, a little longer",
		"media/clip.mp4":        "\x00\x00\x00\x18ftypmp42 a clip",
		"media/capture-03.mov":  pastSendLimit(),
		"media/fixture.zip":     "PK\x03\x04 an archive nobody opens on a phone",
		"docs/retry-policy.pdf": "%PDF-1.4\n% a policy\n",
		"web/icon.png":          pngImage(48, 48, color.RGBA{R: 0x1f, G: 0x7a, B: 0x6a, A: 0xff}),
		"web/new.png":           pngImage(64, 40, color.RGBA{R: 0xf0, G: 0xc6, B: 0x74, A: 0xff}),
		"web/badge.svg":         svgAfter,
		"web/status.html":       statusPage("Upload status"),
	}); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(status.root, "web", "old.png")); err != nil {
		return errors.WithStack(err)
	}
	if err := os.Chmod(filepath.Join(status.root, "bin", "sync"), 0o755); err != nil {
		return errors.WithStack(err)
	}
	paths := []string{"bin", "docs", "media", "web"}
	pageView := map[string]string{"web/status.html": "page"}
	if _, err := w.open(status, "status", "feat/upload-status", "Upload status page and its icon", "A status page for uploads, its icon redrawn, a chime, a demo clip and the policy as a PDF.", paths, pageView); err != nil {
		return err
	}

	// The tag import spec: waiting three hours, then committed, so the
	// project's working copy is clean while the review still reads its round.
	notes, err := w.repo("field-notes", map[string]string{"README.md": "# Field notes\n"})
	if err != nil {
		return err
	}
	w.ago(3 * time.Hour)
	if err := w.edit(notes, map[string]string{"specs/018-tag-import.md": tagImport}); err != nil {
		return err
	}
	note = "The importer now refuses a file with duplicate tags instead of merging them."
	if _, err := w.open(notes, "tags", "spec/018-tag-import", "Spec 018: bulk import for the tag index", note, []string{"specs"}, nil); err != nil {
		return err
	}
	if err := w.commit(notes, "the tag import spec"); err != nil {
		return err
	}

	// A project outside git, with no review.
	_, err = w.plain("scratch")
	return err
}

// worktreeWorld is harbor with a linked worktree beside its main checkout. The
// upload review was captured in the worktree and the drainer in the main
// checkout. With committed set, the worktree's changes are committed, so its
// section lists nothing.
func worktreeWorld(ctx context.Context, committed bool) (*world, error) {
	w, err := newWorld(ctx)
	if err != nil {
		return nil, err
	}
	if err := seedWorktrees(w, committed); err != nil {
		w.close()
		return nil, err
	}
	w.at = now
	return w, nil
}

// seedWorktrees writes the two working copies and a review from each.
func seedWorktrees(w *world, committed bool) error {
	harbor, err := w.repo("harbor", map[string]string{
		"docs/architecture.md":      archBefore,
		"internal/outbox/drain.go":  drainBefore,
		"internal/upload/worker.go": workerBefore,
	})
	if err != nil {
		return err
	}
	retry, err := w.worktree(harbor, "harbor-retry")
	if err != nil {
		return err
	}
	w.ids["harbor-retry copy"] = retry.copyID

	w.ago(2 * time.Hour)
	if err := w.edit(harbor, map[string]string{"internal/outbox/drain.go": drainRound1}); err != nil {
		return err
	}
	drain, err := w.open(harbor, "drain", "fix/outbox-drain", "Offline outbox drainer", "A first cut, to agree the shape.", []string{"internal/outbox"}, nil)
	if err != nil {
		return err
	}
	w.ago(26 * time.Minute)
	if err := w.changes(drain); err != nil {
		return err
	}

	w.ago(61 * time.Minute)
	if err := w.edit(retry, map[string]string{"internal/upload/worker.go": workerAfter, "internal/upload/backoff.go": backoffGo}); err != nil {
		return err
	}
	note := "The worker retries three times with backoff, then gives up and records the failure on the job."
	if _, err := w.open(retry, "upload", "feat/upload-retry", "Upload worker and its retry ladder", note, []string{"internal/upload"}, nil); err != nil {
		return err
	}
	// The main checkout has changes no review holds.
	if err := w.edit(harbor, map[string]string{"internal/outbox/drain_test.go": drainTest, "docs/architecture.md": archAfter}); err != nil {
		return err
	}
	if committed {
		return w.commit(retry, "the retry ladder")
	}
	return nil
}
