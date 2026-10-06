// Command seed writes a populated state directory for looking at the surface:
// three projects with repositories, reviews waiting, worked on and settled,
// rounds decided and not, comments early and late, and the agent's replies.
//
// It writes through the store and capture, because nothing on the socket can
// decide a round. Every request, verdict and comment is backdated through the
// store's clock, so ages read as the tearouts' do. A daemon serves it with
// XDG_STATE_HOME pointing at the directory and --surface-addr on loopback,
// apart from any daemon agents are using:
//
//	go run ./tool/seed -dir /tmp/eyeball-seed
//	XDG_STATE_HOME=/tmp/eyeball-seed EYEBALL_NO_START=1 go run ./cmd/eyeball serve --surface-addr 127.0.0.1:8111
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/daemon"
	"github.com/noonat/eyeball/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed: %v\n", err)
		os.Exit(1)
	}
}

// run is everything main does, so a failure returns rather than exiting.
func run() error {
	dir := flag.String("dir", "", "the directory to seed, which becomes XDG_STATE_HOME; it must not exist yet")
	empty := flag.Bool("nothing-waiting", false, "answer every waiting round, for the queue's empty state")
	flag.Parse()
	if *dir == "" {
		return errors.New("pass -dir, a directory that does not exist yet")
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		return errors.WithStack(err)
	}
	s, err := newSeed(context.Background(), root)
	if err != nil {
		return err
	}
	defer func() { _ = s.store.Close() }()
	if err := s.write(); err != nil {
		return err
	}
	if *empty {
		if err := s.answerAll(); err != nil {
			return err
		}
	}
	fmt.Printf("seeded %s\nserve it with:\n  XDG_STATE_HOME=%s EYEBALL_NO_START=1 go run ./cmd/eyeball serve --surface-addr 127.0.0.1:8111\n", root, root)
	return nil
}

// seed holds what the writing needs.
type seed struct {
	ctx   context.Context
	root  string
	store *store.Store
	blobs *blob.Store
	// now is when the seed runs, and at the time the store stamps next.
	now, at time.Time
}

// newSeed prepares the state directory and opens its store on a clock the
// seed moves.
func newSeed(ctx context.Context, root string) (*seed, error) {
	// A directory that exists may be somebody's state. Refusing it is what
	// keeps a seed from writing into the daemon agents are using.
	if _, err := os.Stat(root); err == nil {
		return nil, errors.Newf("%s exists; seed writes only a directory of its own", root)
	}
	paths, err := daemon.Locate(func(name string) string {
		if name == "XDG_STATE_HOME" {
			return root
		}
		return ""
	}, root)
	if err != nil {
		return nil, err
	}
	if err := daemon.Prepare(paths); err != nil {
		return nil, err
	}
	s := &seed{ctx: ctx, root: root, now: time.Now().UTC()}
	s.at = s.now
	if s.store, err = store.OpenWithClock(ctx, paths.DB, func() time.Time { return s.at }); err != nil {
		return nil, err
	}
	if s.blobs, err = blob.Open(paths.Blobs); err != nil {
		_ = s.store.Close()
		return nil, err
	}
	return s, nil
}

// ago sets the clock to d before the seed's start.
func (s *seed) ago(d time.Duration) {
	s.at = s.now.Add(-d)
}

// project is one seeded repository.
type project struct {
	id   int64
	root string
	base string
}

// review is one seeded review's latest state.
type review struct {
	id     int64
	paths  []string
	round  store.Round
	files  []store.File
	number int
}

// write seeds every project and review. Each review's actions are written in
// the order they happened, each backdated to when it happened, and the reviews
// are written one after another, so the clock jumps between them.
func (s *seed) write() error {
	eyeball, err := s.repo("eyeball", map[string]string{
		"docs/product.md": productBefore,
		"docs/voice.md":   "# Voice\n\nPlain, simple English.\n",
		"web/queue.html":  "<main></main>\n",
	})
	if err != nil {
		return err
	}
	harbor, err := s.repo("harbor", map[string]string{
		"internal/storage/sync.go":  syncBefore,
		"internal/upload/worker.go": workerBefore,
		"internal/outbox/drain.go":  "package outbox\n\nfunc Drain() {}\n",
	})
	if err != nil {
		return err
	}
	// The spec is not in the base, so its review is one new document, which
	// the review page shows on the page rather than in a card.
	notes, err := s.repo("field-notes", map[string]string{
		"README.md": "# Field notes\n",
	})
	if err != nil {
		return err
	}

	// Settled yesterday: the voice document.
	s.ago(30 * time.Hour)
	s.edit(eyeball, "docs/voice.md", "# Voice\n\nPlain, simple English. Short sentences, one idea each.\n\nLead with the answer, then explain it.\n")
	voice, err := s.open(eyeball, "spec/012-surface", "Voice, both kinds", []string{"docs/voice.md"}, "The prose voice and the conversational one, written down.")
	if err != nil {
		return err
	}
	s.ago(26 * time.Hour)
	if err := s.store.Decide(s.ctx, voice.round.ID, store.VerdictApprove, ""); err != nil {
		return err
	}

	// The product document: round 1 asked for changes, round 2 waits.
	s.ago(8 * time.Hour)
	s.edit(eyeball, "docs/product.md", productRound1)
	product, err := s.open(eyeball, "spec/012-surface", "Product doc, second pass", []string{"docs/product.md"}, "First pass at the widening section.\n\nThe compare control is the part worth arguing about.")
	if err != nil {
		return err
	}
	s.ago(7 * time.Hour)
	named, err := s.comment(product, store.NewComment{Level: store.LevelLine, Path: "docs/product.md", StartLine: 5, EndLine: 5, Quoted: "Widening is one control, not a navigation.", Body: "This reads as a rule but the section never says what the control looks like. Name it."})
	if err != nil {
		return err
	}
	moved, err := s.comment(product, store.NewComment{Level: store.LevelReview, Body: "The base-is-empty case is buried in Open questions. It is a behavior, not a question."})
	if err != nil {
		return err
	}
	s.ago(6 * time.Hour)
	if err := s.store.Decide(s.ctx, product.round.ID, store.VerdictChanges, "Two things, both small."); err != nil {
		return err
	}
	s.ago(5 * time.Hour)
	if err := s.store.Reply(s.ctx, named.ID, "Named it. The section now describes a segmented control with two settings."); err != nil {
		return err
	}
	if err := s.store.Reply(s.ctx, moved.ID, "Moved into A round is frozen, with the cost of running outside git stated."); err != nil {
		return err
	}
	s.ago(12 * time.Minute)
	s.edit(eyeball, "docs/product.md", productRound2)
	if err := s.next(eyeball, &product, "Both comments from round 1 are done. The widening section now names the control instead of describing the behavior.\n\nStill unsure the compare control needs two settings."); err != nil {
		return err
	}

	// The queue markup: changes requested four minutes ago.
	s.ago(time.Hour)
	s.edit(eyeball, "web/queue.html", "<main>\n  <section class=\"waiting\"></section>\n  <section class=\"working\"></section>\n</main>\n")
	queue, err := s.open(eyeball, "spec/014-queue", "Queue screen markup", []string{"web/queue.html"}, "The three sections, empty.")
	if err != nil {
		return err
	}
	s.ago(4 * time.Minute)
	if err := s.store.Decide(s.ctx, queue.round.ID, store.VerdictChanges, "Settled is missing."); err != nil {
		return err
	}

	// The delta sync watermark: approved two hours ago, with a late comment.
	s.ago(3 * time.Hour)
	s.edit(harbor, "internal/storage/sync.go", syncAfter)
	delta, err := s.open(harbor, "fix/outbox-drain", "Delta sync watermark", []string{"internal/storage/sync.go"}, "Keeps the watermark from moving backwards when the clock does.")
	if err != nil {
		return err
	}
	s.ago(150 * time.Minute)
	comments := []store.NewComment{
		{Level: store.LevelLine, Path: "internal/storage/sync.go", StartLine: 9, EndLine: 11, Quoted: "if now.Before(w.last) {\n\t\tnow = w.last\n\t}", Body: "This is the part that worried me. Good."},
		{Level: store.LevelFile, Path: "internal/storage/sync.go", Body: "The test covers the backwards clock."},
		{Level: store.LevelReview, Body: "Ship it."},
	}
	for _, c := range comments {
		if _, err := s.comment(delta, c); err != nil {
			return err
		}
	}
	s.ago(2 * time.Hour)
	if err := s.store.Decide(s.ctx, delta.round.ID, store.VerdictApprove, "Good. The clamp on a clock that has gone backwards is the part that worried me, and the test covers it."); err != nil {
		return err
	}
	s.ago(time.Hour)
	if _, err := s.comment(delta, store.NewComment{Level: store.LevelReview, Body: "One more thing: log when the clamp fires."}); err != nil {
		return err
	}

	// The upload worker: waiting an hour, a Go diff with word marks.
	s.ago(61 * time.Minute)
	s.edit(harbor, "internal/upload/worker.go", workerAfter)
	if _, err := s.open(harbor, "feat/upload-retry", "Upload worker and its retry ladder", []string{"internal/upload"}, "The worker retries with backoff now, and gives up when the context ends.\n\nThe backoff numbers are guesses."); err != nil {
		return err
	}

	// The outbox drainer: changes requested 26 minutes ago.
	s.ago(50 * time.Minute)
	s.edit(harbor, "internal/outbox/drain.go", "package outbox\n\n// Drain sends what queued while offline.\nfunc Drain() error {\n\treturn nil\n}\n")
	drain, err := s.open(harbor, "fix/outbox-drain", "Offline outbox drainer", []string{"internal/outbox"}, "A first cut, to agree the shape.")
	if err != nil {
		return err
	}
	s.ago(26 * time.Minute)
	if err := s.store.Decide(s.ctx, drain.round.ID, store.VerdictChanges, "It drains nothing yet."); err != nil {
		return err
	}

	// The tag import spec: waiting three hours.
	s.ago(3 * time.Hour)
	s.edit(notes, "specs/018-tag-import.md", tagImport)
	if _, err := s.open(notes, "spec/018-tag-import", "Spec 018: bulk import for the tag index", []string{"specs"}, "New spec, nothing to diff against. The open question is iteration 3: whether an import that partially fails should roll back or report per row."); err != nil {
		return err
	}
	return nil
}

// answerAll requests changes on every waiting round a minute ago, which leaves
// the queue with nothing waiting and agents still at work.
func (s *seed) answerAll() error {
	s.ago(time.Minute)
	q, err := s.store.Queue(s.ctx, 5)
	if err != nil {
		return err
	}
	for _, row := range q.Waiting {
		round, err := s.store.Round(s.ctx, row.ReviewID, row.Round)
		if err != nil {
			return err
		}
		if err := s.store.Decide(s.ctx, round.ID, store.VerdictChanges, "Not yet."); err != nil {
			return err
		}
	}
	return nil
}

// repo makes a git repository with files committed as its base.
func (s *seed) repo(name string, files map[string]string) (project, error) {
	root := filepath.Join(s.root, "repos", name)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return project{}, errors.WithStack(err)
	}
	if _, err := git(root, "init", "--quiet", "--template=", "-b", "main"); err != nil {
		return project{}, err
	}
	for path, content := range files {
		s.edit(project{root: root}, path, content)
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
	p, err := s.store.EnsureProject(s.ctx, root, name)
	if err != nil {
		return project{}, err
	}
	return project{id: p.ID, root: root, base: base}, nil
}

// edit writes one file into a project's working copy. A write that fails
// leaves a file missing, which the capture after it reports.
func (s *seed) edit(p project, path, content string) {
	full := filepath.Join(p.root, filepath.FromSlash(path))
	_ = os.MkdirAll(filepath.Dir(full), 0o700)
	_ = os.WriteFile(full, []byte(content), 0o600)
}

// open captures a first round and opens a review on it.
func (s *seed) open(p project, agent, title string, paths []string, note string) (review, error) {
	req, err := capture.Freeze(s.ctx, capture.Options{Root: p.root, Base: p.base, Paths: paths, Round: 1, Blobs: s.blobs})
	if err != nil {
		return review{}, errors.Wrapf(err, "capture %s", title)
	}
	req.Note = note
	r, round, err := s.store.OpenReview(s.ctx, store.NewReview{ProjectID: p.id, Agent: agent, Title: title, Paths: paths, Request: req})
	if err != nil {
		return review{}, errors.Wrapf(err, "open %s", title)
	}
	return review{id: r.ID, paths: paths, round: round, files: req.Files, number: 1}, nil
}

// next captures a review's next round.
func (s *seed) next(p project, r *review, note string) error {
	req, err := capture.Freeze(s.ctx, capture.Options{Root: p.root, Base: p.base, Paths: r.paths, Round: r.number + 1, Previous: r.files, Blobs: s.blobs})
	if err != nil {
		return errors.Wrap(err, "capture the next round")
	}
	req.Note = note
	round, err := s.store.OpenRound(s.ctx, r.id, req)
	if err != nil {
		return err
	}
	r.round, r.files, r.number = round, req.Files, r.number+1
	return nil
}

// comment writes a comment on a review's latest round.
func (s *seed) comment(r review, c store.NewComment) (store.Comment, error) {
	c.RoundID = r.round.ID
	return s.store.AddComment(s.ctx, c)
}

// git runs one git command in dir with no configuration but its own.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(
		os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=eyeball seed",
		"GIT_AUTHOR_EMAIL=seed@example.invalid",
		"GIT_COMMITTER_NAME=eyeball seed",
		"GIT_COMMITTER_EMAIL=seed@example.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), out)
	}
	return strings.TrimSpace(string(out)), nil
}

// The seeded files' contents, before and after.
const (
	productBefore = "# Product\n\n## Widening\n\nThree steps out.\n"
	productRound1 = "# Product\n\n## Widening\n\nWidening is one control, not a navigation.\n\nThree steps out, each one control rather than a navigation.\n\n## Open questions\n\nWhat happens when the base is empty?\n"
	productRound2 = "# Product\n\n## Widening\n\nThree steps out, each one control rather than a navigation.\n\n**From a hunk to the whole file**, with the change still marked in place.\n\n**What the change is measured against is one segmented control with two settings.** The previous round is the default from round two onward.\n\n## A round is frozen\n\nAn empty base compares with nothing, so every line reads as added.\n"
	tagImport     = "# Bulk import for the tag index\n\nTagging a season's worth of notes one at a time is the reason the index sits half empty. Import takes a manifest and adds what is missing.\n\n## Iteration 1: the manifest format\n\n- Read a manifest from a path or from stdin\n- Skip a tag already present by name or by slug\n- Report added and skipped separately\n"
	syncBefore    = "package storage\n\nimport \"time\"\n\n// watermark is the newest time synced.\ntype watermark struct{ last time.Time }\n\nfunc (w *watermark) advance(now time.Time) {\n\tw.last = now\n}\n"
	syncAfter     = "package storage\n\nimport \"time\"\n\n// watermark is the newest time synced.\ntype watermark struct{ last time.Time }\n\nfunc (w *watermark) advance(now time.Time) {\n\tif now.Before(w.last) {\n\t\tnow = w.last\n\t}\n\tw.last = now\n}\n"
	workerBefore  = "package upload\n\nimport \"context\"\n\n// Worker uploads one file.\ntype Worker struct{ maxTries int }\n\nfunc (w *Worker) run(ctx context.Context) error {\n\treturn w.send(ctx)\n}\n\nfunc (w *Worker) send(ctx context.Context) error { return nil }\n"
	workerAfter   = "package upload\n\nimport (\n\t\"context\"\n\t\"time\"\n)\n\n// Worker uploads one file.\ntype Worker struct{ maxTries int }\n\nfunc (w *Worker) run(ctx context.Context) error {\n\tfor attempt := 0; attempt < w.maxTries; attempt++ {\n\t\tif err := w.send(ctx); err == nil {\n\t\t\treturn nil\n\t\t}\n\t\tselect {\n\t\tcase <-ctx.Done():\n\t\t\treturn ctx.Err()\n\t\tcase <-time.After(backoff(attempt)):\n\t\t}\n\t}\n\treturn w.send(ctx)\n}\n\nfunc backoff(attempt int) time.Duration { return time.Duration(1<<attempt) * time.Second }\n\nfunc (w *Worker) send(ctx context.Context) error { return nil }\n"
)
