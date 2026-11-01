package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/store"
)

// pageState is one review page, read as its template draws it. Only the markup
// is written, as a component's story compares itself with it.
type pageState struct {
	// file is the name the state's files share, and world the world it is read
	// from.
	file, world string
	// address is the page's path and query.
	address func(ids map[string]int64) string
}

// writeState is one answer the template routes give to a write or a fragment
// request: a composer, a comment card, the decision bar. It is written as it
// was sent, with its status in the name when it is not 200.
type writeState struct {
	// file and world are as in pageState.
	file, world string
	// method is the request's method.
	method string
	// address is the request's path and query.
	address func(ids map[string]int64) string
	// form is the request's body, and nil for none.
	form func(ids map[string]int64) url.Values
	// keep names the comment the write stored, as review-key, so a later write
	// can address it.
	keep string
}

// reviewPages are the states of the review page. The frames of
// docs/design/review.html each have one, or are the same page in another
// theme or with a control opened in the browser: a fold opened, a line selected,
// the composer open and the review box are the page of one of these with
// script on it.
var reviewPages = []pageState{
	// The frames "a second round", "a fold opened", "a changed list item" and
	// "what words cannot mark" are documents folded to what changed.
	{file: "review-doc", world: "review", address: pageOf("product", "")},
	{file: "review-doc-whole", world: "review", address: pageOf("product", "?against=base")},
	{file: "review-doc-round1", world: "review", address: pageOf("product", "/rounds/1")},
	{file: "review-code", world: "review", address: pageOf("upload", "")},
	{file: "review-approved", world: "review", address: pageOf("approved", "")},
	{file: "review-deferred", world: "review", address: pageOf("deferred", "")},
	{file: "review-held", world: "review", address: pageOf("held", "")},
	{file: "review-listed", world: "review", address: pageOf("listed", "")},
	{file: "review-fresh", world: "review", address: pageOf("fresh", "")},
	{file: "review-unmeasurable", world: "review", address: pageOf("lost", "")},
	{file: "review-abandoned", world: "review", address: pageOf("withdrawn", "")},
	{file: "review-cards", world: "week", address: pageOf("status", "")},
	{file: "review-earlier", world: "week", address: pageOf("drain", "/rounds/1")},
}

// reviewWrites are the fragments and the writes, in the order they are sent.
// A write changes what the next one sees, so each group is ordered: the reads
// first, then the refusals that need a review with no comments, then the
// writes that add and change comments.
var reviewWrites = []writeState{
	{file: "review-composer-review", world: "review", method: "GET", address: composerAt("upload", "level=review&round=1")},
	{file: "review-composer-file", world: "review", method: "GET", address: composerAt("upload", "level=file&round=1&path=internal/upload/backoff.go")},
	{file: "review-composer-line", world: "review", method: "GET", address: composerAt("upload", "level=line&round=1&path=internal/upload/worker.go&start=13")},
	{file: "review-composer-line-empty", world: "review", method: "GET", address: composerAt("upload", "level=line&round=1&path=internal/upload/worker.go&start=12")},
	{file: "review-composer-removed", world: "review", method: "GET", address: composerAt("upload", "level=file&round=1&path=internal/upload/worker.go&oldline=13")},
	{file: "review-composer-passage", world: "review", method: "GET", address: composerAt("product", "level=passage&round=2&path=docs/product.md&start=9&end=9")},
	{file: "review-composer-passage-commented", world: "review", method: "GET", address: composerAt("product", "level=passage&round=2&path=docs/product.md&start=7&end=7")},
	{file: "review-composer-whole", world: "review", method: "GET", address: composerAt("product", "level=file&round=2&path=docs/product.md&against=base")},
	{file: "review-composer-late", world: "review", method: "GET", address: composerAt("approved", "level=file&round=1&path=internal/approved/limit.go")},
	{file: "review-composer-late-review", world: "review", method: "GET", address: composerAt("approved", "level=review&round=1")},
	{file: "review-composer-bad", world: "review", method: "GET", address: composerAt("upload", "level=line&round=1&path=internal/upload/worker.go")},
	{file: "review-composer-round", world: "review", method: "GET", address: composerAt("upload", "level=review&round=9")},
	{file: "review-comment", world: "review", method: "GET", address: commentAt("upload-line", "")},
	{file: "review-comment-edit", world: "review", method: "GET", address: commentAt("upload-line", "?edit=1")},
	{file: "review-comment-late", world: "review", method: "GET", address: commentAt("approved-late", "")},
	{file: "review-comment-late-edit", world: "review", method: "GET", address: commentAt("approved-late", "?edit=1")},
	{file: "review-comment-released", world: "review", method: "GET", address: commentAt("approved-sent", "")},
	{file: "review-comment-released-edit", world: "review", method: "GET", address: commentAt("approved-sent", "?edit=1")},
	{file: "review-comment-missing", world: "review", method: "GET", address: commentAt("upload-none", "")},

	{file: "review-decide-refused", world: "review", method: "POST", address: decisionAt("forms"), form: verdictForm("forms", 1, "changes", "")},
	{file: "review-decide-refused-note", world: "review", method: "POST", address: decisionAt("forms"), form: verdictForm("forms", 1, "changes", "  ")},
	{file: "review-decide-stale", world: "review", method: "POST", address: decisionAt("approved"), form: verdictForm("approved", 1, "approve", "")},
	{file: "review-decide-stale-round", world: "review", method: "POST", address: decisionAt("forms"), form: verdictForm("forms", 9, "approve", "")},
	{file: "review-decide-bad", world: "review", method: "POST", address: decisionAt("forms"), form: verdictForm("forms", 1, "maybe", "")},

	{file: "review-add-empty", world: "review", method: "POST", address: commentsAt("forms"), form: commentForm("forms", 1, "review", "", 0, 0, "  ")},
	{file: "review-add-review", world: "review", method: "POST", address: commentsAt("forms"), form: commentForm("forms", 1, "review", "", 0, 0, "Say what the failure looks like."), keep: "forms-review"},
	{file: "review-add-file", world: "review", method: "POST", address: commentsAt("forms"), form: commentForm("forms", 1, "file", "internal/forms/a.go", 0, 0, "This file wants a test."), keep: "forms-file"},
	{file: "review-add-line", world: "review", method: "POST", address: commentsAt("forms"), form: commentForm("forms", 1, "line", "internal/forms/a.go", 6, 0, "Why not return the error?")},
	{file: "review-add-line-second", world: "review", method: "POST", address: commentsAt("forms"), form: commentForm("forms", 1, "line", "internal/forms/a.go", 6, 0, "A second thought on the same line.")},
	{file: "review-add-passage", world: "review", method: "POST", address: commentsAt("forms"), form: commentForm("forms", 1, "passage", "internal/forms/doc/forms.md", 7, 7, "This sentence needs a number.")},
	{file: "review-add-removed", world: "review", method: "POST", address: commentsAt("forms"), form: commentForm("forms", 1, "file", "internal/forms/a.go", 0, 0, "A removed line.", "oldline", "3")},
	{file: "review-add-bad", world: "review", method: "POST", address: commentsAt("forms"), form: commentForm("forms", 1, "line", "internal/forms/a.go", 99, 0, "No such line.")},
	{file: "review-add-late", world: "review", method: "POST", address: commentsAt("approved"), form: commentForm("approved", 1, "file", "internal/approved/limit.go", 0, 0, "Sent at once.")},
	{file: "review-add-no-round", world: "review", method: "POST", address: commentsAt("forms"), form: commentForm("forms", 9, "review", "", 0, 0, "Nowhere to go.")},
	{file: "review-edit", world: "review", method: "PUT", address: commentAt("forms-file", ""), form: bodyForm("A sharper note on the file.")},
	{file: "review-edit-empty", world: "review", method: "PUT", address: commentAt("forms-file", ""), form: bodyForm("")},
	{file: "review-edit-released", world: "review", method: "PUT", address: commentAt("approved-sent", ""), form: bodyForm("Too late.")},
	{file: "review-delete", world: "review", method: "DELETE", address: commentAt("forms-review", "")},
	{file: "review-delete-released", world: "review", method: "DELETE", address: commentAt("approved-sent", "")},
	{file: "review-delete-again", world: "review", method: "DELETE", address: commentAt("forms-review", "")},
	{file: "review-decide-approve", world: "review", method: "POST", address: decisionAt("forms"), form: verdictForm("forms", 1, "approve", "")},
}

// pageOf is a review's page, with a suffix for a round or a query.
func pageOf(review, suffix string) func(ids map[string]int64) string {
	return func(ids map[string]int64) string {
		path, query, _ := strings.Cut(suffix, "?")
		if query != "" {
			query = "?" + query
		}
		return fmt.Sprintf("/reviews/%d%s%s", ids[review], path, query)
	}
}

// composerAt is the composer route, with a query naming the anchor.
func composerAt(review, query string) func(ids map[string]int64) string {
	return func(ids map[string]int64) string {
		return fmt.Sprintf("/reviews/%d/composer?%s", ids[review], query)
	}
}

// commentAt is a comment's own address, the review being the one the comment's
// key starts with.
func commentAt(key, suffix string) func(ids map[string]int64) string {
	return func(ids map[string]int64) string {
		return fmt.Sprintf("/reviews/%d/comments/%d%s", ids[strings.SplitN(key, "-", 2)[0]], ids["comment:"+key], suffix)
	}
}

// commentsAt is where a review's comments post.
func commentsAt(review string) func(ids map[string]int64) string {
	return func(ids map[string]int64) string {
		return fmt.Sprintf("/reviews/%d/comments", ids[review])
	}
}

// decisionAt is where a review's verdict posts.
func decisionAt(review string) func(ids map[string]int64) string {
	return func(ids map[string]int64) string {
		return fmt.Sprintf("/reviews/%d/decision", ids[review])
	}
}

// verdictForm is the form the decision bar sends.
func verdictForm(review string, round int, verdict, note string) func(ids map[string]int64) url.Values {
	return func(ids map[string]int64) url.Values {
		return url.Values{"round": {fmt.Sprint(round)}, "verdict": {verdict}, "note": {note}}
	}
}

// commentForm is the form a composer sends. Extra pairs are further fields.
func commentForm(review string, round int, level, path string, start, end int, body string, extra ...string) func(ids map[string]int64) url.Values {
	return func(ids map[string]int64) url.Values {
		form := url.Values{"round": {fmt.Sprint(round)}, "level": {level}, "body": {body}}
		if path != "" {
			form.Set("path", path)
		}
		if start != 0 {
			form.Set("start", fmt.Sprint(start))
		}
		if end != 0 {
			form.Set("end", fmt.Sprint(end))
		}
		for i := 0; i+1 < len(extra); i += 2 {
			form.Set(extra[i], extra[i+1])
		}
		return form
	}
}

// bodyForm is the form an edit sends.
func bodyForm(body string) func(ids map[string]int64) url.Values {
	return func(ids map[string]int64) url.Values {
		return url.Values{"body": {body}}
	}
}

// The files of the review world.
const (
	limitBefore = "package approved\n\nconst limit = 3\n"
	limitAfter  = "package approved\n\nconst limit = 5\n"
	guideBefore = "# Guide\n\n## Setup\n\nInstall the tool.\n\n## Usage\n\nRun it once.\n\n## Limits\n\nThere are none.\n"
	guideAfter  = "# Guide\n\n## Setup\n\nInstall the tool.\n\n## Usage\n\nRun it twice, then check the log.\n\n## Limits\n\nThere are none.\n"
	bigBefore   = "package deferred\n\n// Base is the first step.\nfunc Base() int { return 0 }\n"
	smallBefore = "package deferred\n\nconst name = \"small\"\n"
	smallAfter  = "package deferred\n\nconst name = \"smaller\"\n"
	heldBefore  = "package held\n\n// Base is the first step.\nfunc Base() int { return 0 }\n"
	tinyBefore  = "package held\n\nconst tiny = 1\n"
	tinyAfter   = "package held\n\nconst tiny = 2\n"
	formsBefore = "package forms\n\nfunc Run() error {\n\treturn nil\n}\n"
	formsAfter  = "package forms\n\nimport \"errors\"\n\nfunc Run() error {\n\treturn errors.New(\"later\")\n}\n"
	formsDocOld = "# Forms\n\nEvery form posts once.\n\nA retry sends the same key.\n\nThe last paragraph.\n"
	formsDocNew = "# Forms\n\nEvery form posts once.\n\nA retry sends the same key.\n\nA key that was stored stores nothing new.\n\nThe last paragraph.\n"
	freshDoc    = "# The plan\n\nThe first round has an empty base, so nothing is marked.\n\n## Steps\n\nWrite it. Read it. Ship it.\n"
	lostBefore  = "package lost\n\nconst a = 1\n"
	lostAfter   = "package lost\n\nconst a = 2\n"
	withdrawn   = "package withdrawn\n\nconst b = 2\n"
)

// steps is n functions of two lines each, which a file grows by to move as
// many lines.
func steps(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "// step%d is step %d.\nfunc step%d() int { return %d }\n", i, i, i, i)
	}
	return b.String()
}

// reviewWorld is the review page's states: a document in its second round, a
// code review, a decided one, a large one, a round whose base is gone and a
// review the agent withdrew.
func reviewWorld(ctx context.Context) (*world, error) {
	w, err := newWorld(ctx)
	if err != nil {
		return nil, err
	}
	if err := seedReview(w); err != nil {
		w.close()
		return nil, err
	}
	w.at = now
	return w, nil
}

// comment writes a comment on a round and keeps its id under a key the
// states name it by.
func (w *world) comment(r round, key string, nc store.NewComment) error {
	nc.RoundID = r.id
	c, err := w.db.AddComment(w.ctx, nc)
	if err != nil {
		return errors.Wrapf(err, "comment %s", key)
	}
	w.ids["comment:"+key] = c.ID
	return nil
}

// seedReview writes every project and review of the review world.
func seedReview(w *world) error {
	steps320, steps600 := steps(160), steps(300)
	listed := map[string]string{}
	changed := map[string]string{}
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("internal/listed/f%02d.go", i)
		listed[name] = fmt.Sprintf("package listed\n\nconst f%d = 0\n", i)
		changed[name] = fmt.Sprintf("package listed\n\nconst f%d = %d\n", i, i)
	}
	files := map[string]string{
		"internal/upload/worker.go":   workerBefore,
		"internal/approved/limit.go":  limitBefore,
		"internal/deferred/big.go":    bigBefore,
		"internal/deferred/small.go":  smallBefore,
		"internal/deferred/guide.md":  guideBefore,
		"internal/held/gen.go":        heldBefore,
		"internal/held/tiny.go":       tinyBefore,
		"internal/forms/a.go":         formsBefore,
		"internal/forms/doc/forms.md": formsDocOld,
		"internal/withdrawn/w.go":     "package withdrawn\n",
	}
	for name, content := range listed {
		files[name] = content
	}
	harbor, err := w.repo("harbor", files)
	if err != nil {
		return err
	}

	// The product document: round 1 asked for changes on its comments and the
	// agent answered them, and round 2 waits with comments of its own.
	eyeball, err := w.repo("eyeball", map[string]string{"docs/product.md": productStart, "docs/voice.md": "# Voice\n\nPlain, simple English.\n"})
	if err != nil {
		return err
	}
	w.ago(8 * time.Hour)
	if err := w.edit(eyeball, map[string]string{"docs/product.md": productOne}); err != nil {
		return err
	}
	paths := []string{"docs/product.md"}
	first, err := w.open(eyeball, "product", "spec/012-surface", "Product doc, second pass", "First pass at the widening section.", paths, nil)
	if err != nil {
		return err
	}
	w.ago(7 * time.Hour)
	if err := w.comment(first, "product-review", store.NewComment{Level: store.LevelReview, Body: "The widening section reads as a list of buttons. Say what the reader is trying to do."}); err != nil {
		return err
	}
	if err := w.comment(first, "product-file", store.NewComment{Level: store.LevelFile, Path: "docs/product.md", Quoted: "Open questions", Body: "Move the base question out of here."}); err != nil {
		return err
	}
	w.ago(6 * time.Hour)
	if err := w.changes(first); err != nil {
		return err
	}
	w.ago(5 * time.Hour)
	if err := w.db.Reply(w.ctx, w.ids["comment:product-review"], "The section now opens with the reader's goal."); err != nil {
		return err
	}
	w.ago(12 * time.Minute)
	if err := w.edit(eyeball, map[string]string{"docs/product.md": productTwo}); err != nil {
		return err
	}
	second, err := w.next(eyeball, "product", first, "Both comments from round 1 are done.")
	if err != nil {
		return err
	}
	w.ago(10 * time.Minute)
	if err := w.comment(second, "product-passage", store.NewComment{Level: store.LevelPassage, Path: "docs/product.md", StartLine: 7, EndLine: 7, Quoted: "**From a hunk to the whole file**, with the change still marked in place.", Body: "Name the control here."}); err != nil {
		return err
	}
	if err := w.comment(second, "product-second", store.NewComment{Level: store.LevelReview, Body: "Good. Check the links in See also."}); err != nil {
		return err
	}

	// A code review waiting, with a comment at each level.
	w.ago(61 * time.Minute)
	if err := w.edit(harbor, map[string]string{"internal/upload/worker.go": workerAfter, "internal/upload/backoff.go": backoffGo}); err != nil {
		return err
	}
	note := "The worker retries three times with backoff, then gives up and records the failure on the job."
	upload, err := w.open(harbor, "upload", "feat/upload-retry", "Upload worker and its retry ladder", note, []string{"internal/upload"}, nil)
	if err != nil {
		return err
	}
	w.ago(30 * time.Minute)
	if err := w.comment(upload, "upload-line", store.NewComment{Level: store.LevelLine, Path: "internal/upload/worker.go", StartLine: 13, EndLine: 13, Quoted: "\t\tif err := w.send(ctx); err == nil {", Body: "What does send return on a cancelled context?"}); err != nil {
		return err
	}
	if err := w.comment(upload, "upload-file", store.NewComment{Level: store.LevelFile, Path: "internal/upload/backoff.go", Body: "Is thirty seconds long enough for a cold connection?"}); err != nil {
		return err
	}
	if err := w.comment(upload, "upload-review", store.NewComment{Level: store.LevelReview, Body: "Add a test for the give-up path."}); err != nil {
		return err
	}

	// An approved review with a comment sent with the verdict and one after it.
	w.ago(3 * time.Hour)
	if err := w.edit(harbor, map[string]string{"internal/approved/limit.go": limitAfter}); err != nil {
		return err
	}
	approved, err := w.open(harbor, "approved", "fix/limit", "Raise the retry limit", "The limit goes from three to five.", []string{"internal/approved"}, nil)
	if err != nil {
		return err
	}
	w.ago(2 * time.Hour)
	if err := w.comment(approved, "approved-sent", store.NewComment{Level: store.LevelFile, Path: "internal/approved/limit.go", Body: "Fine, if the backoff caps at thirty seconds."}); err != nil {
		return err
	}
	w.ago(time.Hour)
	if err := w.db.Decide(w.ctx, approved.id, store.VerdictApprove, "Approved with the cap in mind."); err != nil {
		return err
	}
	w.ago(40 * time.Minute)
	if err := w.comment(approved, "approved-late", store.NewComment{Level: store.LevelReview, Body: "One more thing: log the limit at startup."}); err != nil {
		return err
	}

	// A deferred round: a document, a file past the line threshold and a small
	// one, each with comments.
	w.ago(90 * time.Minute)
	if err := w.edit(harbor, map[string]string{
		"internal/deferred/big.go":   bigBefore + steps320,
		"internal/deferred/small.go": smallAfter,
		"internal/deferred/guide.md": guideAfter,
	}); err != nil {
		return err
	}
	deferred, err := w.open(harbor, "deferred", "feat/steps", "Generated steps and a guide", "A long file of steps, a guide that says how to run them and a rename.", []string{"internal/deferred"}, nil)
	if err != nil {
		return err
	}
	w.ago(80 * time.Minute)
	if err := w.comment(deferred, "deferred-line", store.NewComment{Level: store.LevelLine, Path: "internal/deferred/big.go", StartLine: 6, EndLine: 6, Quoted: "func step1() int { return 1 }", Body: "Generate these."}); err != nil {
		return err
	}
	if err := w.comment(deferred, "deferred-file", store.NewComment{Level: store.LevelFile, Path: "internal/deferred/small.go", Body: "Keep the old name."}); err != nil {
		return err
	}
	if err := w.comment(deferred, "deferred-passage", store.NewComment{Level: store.LevelPassage, Path: "internal/deferred/guide.md", StartLine: 9, EndLine: 9, Quoted: "Run it twice, then check the log.", Body: "Which log?"}); err != nil {
		return err
	}

	// A held card: one file moved past the card limit.
	w.ago(100 * time.Minute)
	if err := w.edit(harbor, map[string]string{"internal/held/gen.go": heldBefore + steps600, "internal/held/tiny.go": tinyAfter}); err != nil {
		return err
	}
	if _, err := w.open(harbor, "held", "feat/generated", "A generated file", "The generator's output, checked in.", []string{"internal/held"}, nil); err != nil {
		return err
	}

	// A round of twelve files, which gets a list.
	w.ago(110 * time.Minute)
	if err := w.edit(harbor, changed); err != nil {
		return err
	}
	if _, err := w.open(harbor, "listed", "chore/constants", "Twelve constants", "Each constant takes its number.", []string{"internal/listed"}, nil); err != nil {
		return err
	}

	// A review the agent withdrew, with a comment on it.
	w.ago(5 * time.Hour)
	if err := w.edit(harbor, map[string]string{"internal/withdrawn/w.go": withdrawn}); err != nil {
		return err
	}
	gone, err := w.open(harbor, "withdrawn", "feat/never", "A change nobody needs now", "Superseded.", []string{"internal/withdrawn"}, nil)
	if err != nil {
		return err
	}
	w.ago(4 * time.Hour)
	if err := w.comment(gone, "withdrawn-note", store.NewComment{Level: store.LevelReview, Body: "Is this still wanted?"}); err != nil {
		return err
	}
	w.ago(3 * time.Hour)
	if err := w.db.Abandon(w.ctx, w.ids["withdrawn"]); err != nil {
		return err
	}

	// A review with nothing on it yet, which the writes are tried on.
	w.ago(20 * time.Minute)
	if err := w.edit(harbor, map[string]string{"internal/forms/a.go": formsAfter, "internal/forms/doc/forms.md": formsDocNew}); err != nil {
		return err
	}
	if _, err := w.open(harbor, "forms", "feat/forms", "Forms post once", "Every form posts once and a retry sends the same key.", []string{"internal/forms"}, nil); err != nil {
		return err
	}

	// A review nothing can be compared for: its repository is gone.
	lost, err := w.repo("vanished", map[string]string{"lost.go": lostBefore})
	if err != nil {
		return err
	}
	w.ago(15 * time.Minute)
	if err := w.edit(lost, map[string]string{"lost.go": lostAfter}); err != nil {
		return err
	}
	if _, err := w.open(lost, "lost", "fix/lost", "A change in a repository that is gone", "The repository moved.", []string{"lost.go"}, nil); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(lost.root, ".git")); err != nil {
		return errors.WithStack(err)
	}

	// A new document on its own, in a first round.
	w.ago(25 * time.Minute)
	if err := w.edit(eyeball, map[string]string{"docs/plan.md": freshDoc}); err != nil {
		return err
	}
	if _, err := w.open(eyeball, "fresh", "spec/013-plan", "A new plan", "The plan, as a document.", []string{"docs/plan.md"}, nil); err != nil {
		return err
	}
	return nil
}
