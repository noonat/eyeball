package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/kind"
	"github.com/noonat/eyeball/internal/server"
	"github.com/noonat/eyeball/internal/store"
)

// world is a store of projects with real repositories, behind the server's own
// handlers on the fixed clock. Several scenes read one world, since building a
// repository costs a few git processes.
type world struct {
	ctx   context.Context
	dir   string
	db    *store.Store
	blobs *blob.Store
	srv   *server.Server
	at    time.Time
	// ids holds the id of each project and review by the name the scene uses.
	ids map[string]int64
}

// project is one seeded working copy of a project.
type project struct {
	id   int64
	root string
	base string
	// copyID is the working copy a round is captured from, and zero for the
	// project's own root.
	copyID int64
}

// round is a review's latest round, which the next one is captured against.
type round struct {
	id     int64
	paths  []string
	files  []store.File
	number int
}

// newWorld opens an empty world under a fresh temporary directory, which the
// caller removes. The directory is the server's home, so every path the views
// show is written from it with ~ and does not change between runs.
func newWorld(ctx context.Context) (*world, error) {
	dir, err := os.MkdirTemp("", "fixtures-")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		return nil, errors.WithStack(err)
	}
	w := &world{ctx: ctx, dir: dir, at: now, ids: map[string]int64{}}
	if w.db, err = store.OpenWithClock(ctx, filepath.Join(dir, "eyeball.db"), func() time.Time { return w.at }); err != nil {
		return nil, err
	}
	if w.blobs, err = blob.Open(filepath.Join(dir, "blobs")); err != nil {
		_ = w.db.Close()
		return nil, err
	}
	w.srv = server.New(server.Config{Store: w.db, Blobs: w.blobs, Home: dir, Now: func() time.Time { return w.at }})
	return w, nil
}

// close closes the store and removes the directory.
func (w *world) close() {
	_ = w.db.Close()
	_ = os.RemoveAll(w.dir)
}

// ago sets the clock to d before the instant every age is measured from.
func (w *world) ago(d time.Duration) {
	w.at = now.Add(-d)
}

// get reads an address from the surface and returns the body.
func (w *world) get(address string) ([]byte, error) {
	rec := httptest.NewRecorder()
	w.srv.Surface().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, address, nil))
	if rec.Code != http.StatusOK {
		return nil, errors.Newf("GET %s answered %d", address, rec.Code)
	}
	return rec.Body.Bytes(), nil
}

// repo makes a git repository with files committed as its base.
func (w *world) repo(name string, files map[string]string) (project, error) {
	root := filepath.Join(w.dir, "src", name)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return project{}, errors.WithStack(err)
	}
	if _, err := git(root, "init", "--quiet", "--template=", "-b", "main"); err != nil {
		return project{}, err
	}
	for path, content := range files {
		if err := writeFile(root, path, content); err != nil {
			return project{}, err
		}
	}
	if _, err := git(root, "add", "-A"); err != nil {
		return project{}, err
	}
	if _, err := git(root, "commit", "--quiet", "-m", "base"); err != nil {
		return project{}, err
	}
	base, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return project{}, err
	}
	p, err := w.db.EnsureProject(w.ctx, root, name)
	if err != nil {
		return project{}, err
	}
	w.ids[name] = p.ID
	return project{id: p.ID, root: root, base: base}, nil
}

// plain makes a project outside git: a directory named by a marker file.
func (w *world) plain(name string) (project, error) {
	root := filepath.Join(w.dir, "src", name)
	if err := writeFile(root, capture.Marker, ""); err != nil {
		return project{}, err
	}
	p, err := w.db.EnsureProject(w.ctx, root, name)
	if err != nil {
		return project{}, err
	}
	w.ids[name] = p.ID
	return project{id: p.ID, root: root}, nil
}

// worktree adds a linked worktree to a project's repository, and records the
// main checkout and the worktree as working copies, as a first request from
// each would.
func (w *world) worktree(main project, name string) (project, error) {
	root := filepath.Join(w.dir, "src", name)
	if _, err := git(main.root, "worktree", "add", "--quiet", "-b", name, root); err != nil {
		return project{}, err
	}
	mainCopy, err := w.db.EnsureWorkingCopy(w.ctx, main.id, main.root)
	if err != nil {
		return project{}, err
	}
	if err := w.db.MarkMain(w.ctx, mainCopy.ID); err != nil {
		return project{}, err
	}
	c, err := w.db.EnsureWorkingCopy(w.ctx, main.id, root)
	if err != nil {
		return project{}, err
	}
	return project{id: main.id, root: root, base: main.base, copyID: c.ID}, nil
}

// commit commits every change in a project's working copy.
func (w *world) commit(p project, message string) error {
	if _, err := git(p.root, "add", "-A"); err != nil {
		return err
	}
	_, err := git(p.root, "commit", "--quiet", "-m", message)
	return err
}

// edit writes files into a project's working copy.
func (w *world) edit(p project, files map[string]string) error {
	for path, content := range files {
		if err := writeFile(p.root, path, content); err != nil {
			return err
		}
	}
	return nil
}

// open captures a first round and opens a review on it, which scenes find by
// name.
func (w *world) open(p project, name, agent, title, note string, paths []string, views map[string]string) (round, error) {
	req, err := capture.Freeze(w.ctx, capture.Options{Root: p.root, Base: p.base, Paths: paths, Round: 1, Blobs: w.blobs})
	if err != nil {
		return round{}, errors.Wrapf(err, "capture %s", title)
	}
	req.Note, req.Views, req.WorkingCopyID = note, views, p.copyID
	r, first, err := w.db.OpenReview(w.ctx, store.NewReview{ProjectID: p.id, Agent: agent, Title: title, Paths: paths, Request: req})
	if err != nil {
		return round{}, errors.Wrapf(err, "open %s", title)
	}
	w.ids[name] = r.ID
	return round{id: first.ID, paths: paths, files: req.Files, number: 1}, nil
}

// next captures a review's next round.
func (w *world) next(p project, review string, r round, note string) (round, error) {
	req, err := capture.Freeze(w.ctx, capture.Options{Root: p.root, Base: p.base, Paths: r.paths, Round: r.number + 1, Previous: r.files, Blobs: w.blobs})
	if err != nil {
		return round{}, errors.Wrap(err, "capture the next round")
	}
	req.Note, req.WorkingCopyID = note, p.copyID
	next, err := w.db.OpenRound(w.ctx, w.ids[review], req)
	if err != nil {
		return round{}, err
	}
	return round{id: next.ID, paths: r.paths, files: req.Files, number: r.number + 1}, nil
}

// changes requests changes on a round.
func (w *world) changes(r round) error {
	return w.db.Decide(w.ctx, r.id, store.VerdictChanges, "Not yet.")
}

// writeFile writes one file under a root, making its directory.
func writeFile(root, path, content string) error {
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(os.WriteFile(full, []byte(content), 0o600))
}

// git runs one git command in dir with no configuration but its own, and the
// same commit date every time.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	stamp := now.Add(-48 * time.Hour).Format(time.RFC3339)
	cmd.Env = append(
		os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=eyeball fixtures",
		"GIT_AUTHOR_EMAIL=fixtures@example.invalid",
		"GIT_COMMITTER_NAME=eyeball fixtures",
		"GIT_COMMITTER_EMAIL=fixtures@example.invalid",
		"GIT_AUTHOR_DATE="+stamp,
		"GIT_COMMITTER_DATE="+stamp,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), out)
	}
	return strings.TrimSpace(string(out)), nil
}

// pngImage is a w by h PNG: a field of c with a lighter disc in the middle, so
// a before and an after are told apart at a glance. A caption reads the
// dimensions from its header, which is why the image is a real one.
func pngImage(w, h int, c color.RGBA) string {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	light := color.RGBA{R: c.R/2 + 0x7f, G: c.G/2 + 0x7f, B: c.B/2 + 0x7f, A: 0xff}
	r := min(w, h) / 3
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x-w/2, y-h/2
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, light)
			} else {
				img.Set(x, y, c)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.String()
}

// pastSendLimit is a file one byte over what the surface sends to a phone.
func pastSendLimit() string {
	return strings.Repeat("\x00", kind.MaxSend+1)
}

// The files the worlds hold, before and after.
const (
	archBefore   = "# Architecture\n\nThe worker drains the upload queue.\n\nThe outbox holds what queued while offline.\n"
	archAfter    = "# Architecture\n\nThe worker drains the upload queue, retrying with backoff.\n\nThe outbox holds what queued while offline, and drains on reconnect.\n\nThe cache keeps the last listing in memory.\n"
	drainBefore  = "package outbox\n\nfunc Drain() {}\n"
	drainRound1  = "package outbox\n\n// Drain sends what queued while offline.\nfunc Drain() error {\n\treturn nil\n}\n"
	drainRound2  = "package outbox\n\nimport \"context\"\n\n// Drain sends what queued while offline.\nfunc Drain(ctx context.Context) error {\n\treturn ctx.Err()\n}\n"
	drainTest    = "package outbox\n\nimport \"testing\"\n\nfunc TestDrain(t *testing.T) {\n\tif err := Drain(t.Context()); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n"
	storeGo      = "package storage\n\nimport (\n\t\"context\"\n\t\"database/sql\"\n\n\t\"github.com/cockroachdb/errors\"\n)\n\n// Store is the jobs table.\ntype Store struct{ db *sql.DB }\n\n// Job is one queued upload.\ntype Job struct{ ID int64 }\n\n// Next claims the oldest queued job.\nfunc (s *Store) Next(ctx context.Context) (Job, error) {\n\ttx, err := s.db.BeginTx(ctx, nil)\n\tif err != nil {\n\t\treturn Job{}, errors.Wrap(err, \"begin\")\n\t}\n\tdefer func() { _ = tx.Rollback() }()\n\treturn Job{}, nil\n}\n"
	queueGo      = "package upload\n\nimport \"sync\"\n\n// queue is the jobs waiting to upload.\ntype queue struct {\n\tmu      sync.Mutex\n\tpending []string\n}\n\n// next returns the oldest queued job.\nfunc (q *queue) next() string {\n\tq.mu.Lock()\n\tdefer q.mu.Unlock()\n\treturn q.pending[0]\n}\n"
	backoffGo    = "package upload\n\nimport \"time\"\n\n// backoffCap is the longest a retry waits.\nconst backoffCap = 30 * time.Second\n"
	workerBefore = "package upload\n\nimport \"context\"\n\n// Worker uploads one file.\ntype Worker struct{ maxTries int }\n\nfunc (w *Worker) run(ctx context.Context) error {\n\treturn w.send(ctx)\n}\n\nfunc (w *Worker) send(ctx context.Context) error { return nil }\n"
	workerAfter  = "package upload\n\nimport (\n\t\"context\"\n\t\"time\"\n)\n\n// Worker uploads one file.\ntype Worker struct{ maxTries int }\n\nfunc (w *Worker) run(ctx context.Context) error {\n\tfor attempt := 0; attempt < w.maxTries; attempt++ {\n\t\tif err := w.send(ctx); err == nil {\n\t\t\treturn nil\n\t\t}\n\t\tselect {\n\t\tcase <-ctx.Done():\n\t\t\treturn ctx.Err()\n\t\tcase <-time.After(backoff(attempt)):\n\t\t}\n\t}\n\treturn w.send(ctx)\n}\n\nfunc backoff(attempt int) time.Duration { return time.Duration(1<<attempt) * time.Second }\n\nfunc (w *Worker) send(ctx context.Context) error { return nil }\n"
	syncBefore   = "#!/bin/sh\nexec ./sync --once\n"
	syncAfter    = "#!/bin/sh\nset -eu\nexec ./sync --once\n"
	tagImport    = "# Bulk import for the tag index\n\nTagging a season's worth of notes one at a time is the reason the index sits half empty. Import takes a manifest and adds what is missing.\n\n## Iteration 1: the manifest format\n\n- Read a manifest from a path or from stdin\n- Skip a tag already present by name or by slug\n- Report added and skipped separately\n"
	productStart = "# Product\n\n## Widening\n\nThree steps out.\n"
	productLinks = "## See also\n\nThe voice is in [voice.md](voice.md) and the design in [architecture.md](/docs/architecture.md). [The four levels](#making-a-comment) are above. Questions go to [the tracker](https://example.com/eyeball/issues).\n"
	productOne   = "# Product\n\n## Widening\n\nWidening is one control, not a navigation.\n\nThree steps out, each one control rather than a navigation.\n\nA control offers whatever bases exist.\n\n## Making a comment\n\nA comment attaches at one of four levels. To a line or a range of lines. To a passage, which is what a rendered document offers instead of lines.\n\n## Open questions\n\nWhat happens when the base is empty?\n\n" + productLinks
	productTwo   = "# Product\n\n## Widening\n\nThree steps out, each one control rather than a navigation.\n\n**From a hunk to the whole file**, with the change still marked in place.\n\n**What the change is measured against is one control with two settings.** The previous round is the default from round two onward and answers what moved since the last reading.\n\n## Making a comment\n\nA comment attaches at one of four levels. To a line or a range of\nlines. To a passage, which is what a rendered document offers\ninstead of lines.\n\n## A round is frozen\n\nAn empty base compares with nothing, so every line reads as added.\n\n" + productLinks
	statusCSS    = "body { margin: 0; font: 15px/1.4 system-ui, sans-serif; background: #faf8f5; color: #2b2724 }\nheader { padding: 14px 16px; background: #2b2724; color: #faf8f5; font-weight: 600 }\nsection { margin: 12px 16px; padding: 12px; border: 1px solid #ddd5cc; border-radius: 8px; background: #fff }\n"
	svgBefore    = "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\"><circle cx=\"12\" cy=\"12\" r=\"10\" fill=\"#b04a3c\"/></svg>\n"
	svgAfter     = "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\"><rect x=\"2\" y=\"2\" width=\"20\" height=\"20\" rx=\"5\" fill=\"#1f7a6a\"/></svg>\n"
)

// statusPage is a small page that links its stylesheet beside it.
func statusPage(title string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<!doctype html>\n<meta name=\"viewport\" content=\"width=device-width\">\n<link rel=\"stylesheet\" href=\"status.css\">\n<header>%s</header>\n", title)
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&b, "<section>Job %d: retrying, attempt %d of 3</section>\n", i, i%3+1)
	}
	return b.String()
}

// clipMP4 is three seconds of ffmpeg's test pattern at 320 by 180, H.264
// baseline with its index at the front. It is a copy of tool/seed's, since
// nothing can import a main package, and a real video is the only way to see
// one play.
//
//go:embed testdata/clip.mp4
var clipMP4 string

// wavTone is seconds of a sine at freq hertz, 16-bit mono PCM at rate, fading
// out so the end does not click.
func wavTone(freq, rate, seconds int) string {
	n := rate * seconds
	var buf bytes.Buffer
	le := binary.LittleEndian
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, le, uint32(36+n*2))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(&buf, le, struct {
		Size                      uint32
		Format, Channels          uint16
		Rate, ByteRate            uint32
		BlockAlign, BitsPerSample uint16
	}{Size: 16, Format: 1, Channels: 1, Rate: uint32(rate), ByteRate: uint32(rate * 2), BlockAlign: 2, BitsPerSample: 16})
	buf.WriteString("data")
	_ = binary.Write(&buf, le, uint32(n*2))
	for i := 0; i < n; i++ {
		v := math.Sin(2*math.Pi*float64(freq)*float64(i)/float64(rate)) * (1 - float64(i)/float64(n))
		_ = binary.Write(&buf, le, int16(v*12000))
	}
	return buf.String()
}

// pdfDocument is a PDF with one page for each list of lines, the first line of
// a page its title. The cross-reference offsets are computed so a strict
// reader opens it, and nothing in it varies between runs.
func pdfDocument(pages ...[]string) string {
	// Objects 1 and 2 are the catalog and the page tree, 3 is the font, and
	// each page takes two: the page, then its content stream.
	objects := make([]string, 3+2*len(pages))
	kids := make([]string, len(pages))
	for i, lines := range pages {
		var stream strings.Builder
		for j, line := range lines {
			size, y := 11, 730-(j-1)*18
			if j == 0 {
				size, y = 18, 760
			}
			fmt.Fprintf(&stream, "BT /F1 %d Tf 56 %d Td (%s) Tj ET\n", size, y, line)
		}
		page, content := 4+2*i, 5+2*i
		kids[i] = fmt.Sprintf("%d 0 R", page)
		objects[page-1] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>", content)
		objects[content-1] = fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", stream.Len(), stream.String())
	}
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages))
	objects[2] = "<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman >>"
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.String()
}
