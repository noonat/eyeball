// Command fixtures writes the queue's JSON view for seeded states into
// web/src/fixtures, so a story draws what the API really sends. Beside each view
// it writes the markup the template queue page renders for the same state, which
// the component's story compares itself with, so the two cannot draw the same
// screen differently without a test saying so.
//
// Each state is built in a store of its own through the server's own handler,
// on a fixed clock, so a review's id and a row's age are the same on every run.
// With --check the command writes nothing and fails when a committed file
// differs.
package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/server"
	"github.com/noonat/eyeball/internal/store"
)

// outDir is where the fixtures go, relative to the repository root.
const outDir = "web/src/fixtures"

// now is the instant every age is measured from.
var now = time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "fixtures: %v\n", err)
		os.Exit(1)
	}
}

// run writes the fixtures, or checks them, as its arguments say.
func run(args []string) error {
	check := len(args) == 1 && args[0] == "--check"
	if len(args) > 0 && !check {
		return errors.Newf("usage: fixtures [--check], got %q", strings.Join(args, " "))
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	files, err := Generate(context.Background())
	if err != nil {
		return err
	}
	dir := filepath.Join(root, outDir)
	if !check {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return errors.WithStack(err)
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
	var stale []string
	for name, body := range files {
		have, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(have, body) {
			stale = append(stale, outDir+"/"+name)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		return errors.Newf("stale: %s. Run: make api", strings.Join(stale, ", "))
	}
	return nil
}

// Generate builds every state and returns the view each one answers with, by
// file name.
func Generate(ctx context.Context) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, st := range states {
		body, page, err := render(ctx, st)
		if err != nil {
			return nil, errors.Wrapf(err, "state %s", st.file)
		}
		out[st.file+".json"] = body
		out[st.file+".html"] = page
	}
	return out, nil
}

// render builds one state in a temporary store and reads the queue back through
// GET /api/ and through the template page at GET /.
func render(ctx context.Context, st state) ([]byte, []byte, error) {
	dir, err := os.MkdirTemp("", "fixtures-")
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	defer os.RemoveAll(dir)
	at := now
	clock := func() time.Time { return at }
	db, err := store.OpenWithClock(ctx, filepath.Join(dir, "eyeball.db"), clock)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = db.Close() }()

	projects := map[string]int64{}
	for i, rv := range st.reviews {
		id, ok := projects[rv.project]
		if !ok {
			root := filepath.Join(dir, "src", rv.project)
			if err := os.MkdirAll(root, 0o700); err != nil {
				return nil, nil, errors.WithStack(err)
			}
			p, err := db.EnsureProject(ctx, root, rv.project)
			if err != nil {
				return nil, nil, err
			}
			id = p.ID
			projects[rv.project] = id
		}
		if err := write(ctx, db, &at, id, i, rv); err != nil {
			return nil, nil, errors.Wrapf(err, "review %q", rv.title)
		}
	}
	at = now

	srv := server.New(server.Config{Store: db, Now: clock})
	rec := httptest.NewRecorder()
	srv.Surface().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/", nil))
	if rec.Code != http.StatusOK {
		return nil, nil, errors.Newf("GET /api/ answered %d", rec.Code)
	}
	view := rec.Body.Bytes()

	page := httptest.NewRecorder()
	srv.Surface().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		return nil, nil, errors.Newf("GET / answered %d", page.Code)
	}
	fragment, err := queueMarkup(page.Body.Bytes())
	if err != nil {
		return nil, nil, err
	}
	return view, fragment, nil
}

// write opens one review and answers every round it has had. The clock is
// set before each action, so the rows' ages are exactly the review's.
func write(ctx context.Context, db *store.Store, at *time.Time, project int64, n int, rv review) error {
	claim := fmt.Sprintf("r%d", n+1)
	var files []store.File
	var changes []store.Change
	for i := range rv.files {
		path := fmt.Sprintf("%s/f%d.go", claim, i+1)
		files = append(files, store.File{Path: path, Digest: fmt.Sprintf("%064x", n*100+i+1), Size: 1})
		added, removed := rv.added/rv.files, rv.removed/rv.files
		if i == 0 {
			added, removed = rv.added-added*(rv.files-1), rv.removed-removed*(rv.files-1)
		}
		changes = append(changes, store.Change{Path: path, Added: added, Removed: removed})
	}
	// The latest round was asked for at asked, and answered at answered.
	var asked, answered time.Time
	if rv.verdict == "" {
		asked = now.Add(-rv.ago)
	} else {
		answered = now.Add(-rv.ago)
		asked = answered.Add(-30 * time.Minute)
	}
	request := func(round int) store.Request {
		note := "An earlier round."
		if round == rv.round {
			note = rv.note
		}
		return store.Request{Note: note, Files: files, Changes: changes}
	}
	var id int64
	for round := 1; round <= rv.round; round++ {
		*at = asked.Add(-time.Duration(rv.round-round) * 3 * time.Hour)
		if round == 1 {
			review, _, err := db.OpenReview(ctx, store.NewReview{ProjectID: project, Agent: rv.agent, Title: rv.title, Paths: []string{claim}, Request: request(round)})
			if err != nil {
				return err
			}
			id = review.ID
		} else if _, err := db.OpenRound(ctx, id, request(round)); err != nil {
			return err
		}
		if round == rv.round && rv.verdict == "" {
			return nil
		}
		*at = asked.Add(-time.Duration(rv.round-round)*3*time.Hour + time.Hour)
		if round == rv.round {
			*at = answered
		}
		verdict := store.VerdictChanges
		if round == rv.round {
			verdict = rv.verdict
		}
		r, err := db.Round(ctx, id, round)
		if err != nil {
			return err
		}
		if err := db.Decide(ctx, r.ID, verdict, "A verdict."); err != nil {
			return err
		}
	}
	return nil
}

// repoRoot is the nearest directory above the working directory holding a go.mod.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", errors.WithStack(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above the working directory")
		}
		dir = parent
	}
}
