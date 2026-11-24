package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/store"
	"github.com/noonat/eyeball/internal/view"
)

// sendJSON sends a JSON body to a handler.
func sendJSON(h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return sendRaw(h, method, path, "application/json", string(encoded))
}

// sendRaw sends a body as it is, with the content type given.
func sendRaw(h http.Handler, method, path, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://eyeball"+path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

// readRefusal decodes a refusal and fails on any other body.
func readRefusal(g *WithT, res *httptest.ResponseRecorder) view.Refusal {
	g.THelper()
	g.Expect(res.Header().Get("Content-Type")).To(Equal("application/json"), res.Body.String())
	var out view.Refusal
	dec := json.NewDecoder(res.Body)
	dec.DisallowUnknownFields()
	g.Expect(dec.Decode(&out)).To(Succeed())
	return out
}

// readStored decodes the answer to an added or edited comment.
func readStored(g *WithT, res *httptest.ResponseRecorder) view.CommentStored {
	g.THelper()
	g.Expect(res.Code).To(Equal(http.StatusOK), res.Body.String())
	var out view.CommentStored
	dec := json.NewDecoder(res.Body)
	dec.DisallowUnknownFields()
	g.Expect(dec.Decode(&out)).To(Succeed())
	return out
}

// commentsAPI is the JSON route for a review's comments.
func commentsAPI(review int64) string {
	return "/api/reviews/" + itoa(review) + "/comments"
}

// refusedComments are comments that cannot be stored, on a review of a.txt,
// which reads "one\ntwo\n" at round 1 and "one\n" in the base.
var refusedComments = []struct {
	name    string
	req     view.CommentRequest
	status  int
	message string
}{
	{
		name:    "a line past the end of the file",
		req:     view.CommentRequest{Level: view.CommentLevelLine, Path: "a.txt", Start: 3},
		status:  http.StatusBadRequest,
		message: "a.txt has no line 3.",
	},
	{
		name:    "a path the round cannot read",
		req:     view.CommentRequest{Level: view.CommentLevelLine, Path: "missing.txt", Start: 1},
		status:  http.StatusBadRequest,
		message: "Round 1 holds no readable copy of missing.txt.",
	},
	{
		name:    "a line with no line",
		req:     view.CommentRequest{Level: view.CommentLevelLine, Path: "a.txt"},
		status:  http.StatusBadRequest,
		message: notThisForm,
	},
	{
		name:    "a line with a range",
		req:     view.CommentRequest{Level: view.CommentLevelLine, Path: "a.txt", Start: 1, End: 2},
		status:  http.StatusBadRequest,
		message: notThisForm,
	},
	{
		name:    "a negative line",
		req:     view.CommentRequest{Level: view.CommentLevelLine, Path: "a.txt", Start: -1},
		status:  http.StatusBadRequest,
		message: notThisForm,
	},
	{
		name:    "a negative removed line",
		req:     view.CommentRequest{Level: view.CommentLevelFile, Path: "a.txt", OldLine: -1},
		status:  http.StatusBadRequest,
		message: notThisForm,
	},
	{
		name:    "a review comment naming a path",
		req:     view.CommentRequest{Level: view.CommentLevelReview, Path: "a.txt"},
		status:  http.StatusBadRequest,
		message: notThisForm,
	},
	{
		name:    "a level that does not exist",
		req:     view.CommentRequest{Level: "chapter"},
		status:  http.StatusBadRequest,
		message: notThisForm,
	},
	{
		name:    "no text",
		req:     view.CommentRequest{Level: view.CommentLevelReview, Body: "  "},
		status:  http.StatusBadRequest,
		message: emptyComment,
	},
	{
		name:    "a climb out of the project",
		req:     view.CommentRequest{Level: view.CommentLevelLine, Path: "../x", Start: 1},
		status:  http.StatusBadRequest,
		message: notThisForm,
	},
	{
		name:    "a round the review does not have",
		req:     view.CommentRequest{Level: view.CommentLevelReview, Round: 9},
		status:  http.StatusNotFound,
		message: noRound,
	},
}

func TestServer_addCommentJSON(t *testing.T) {
	cases := []struct {
		name   string
		req    view.CommentRequest
		where  string
		quoted string
		// level, start and end are what the store holds, which the answer does not
		// carry.
		level      store.Level
		start, end int
	}{
		{
			name:   "a line",
			req:    view.CommentRequest{Level: view.CommentLevelLine, Path: "a.txt", Start: 2},
			where:  "a.txt:2",
			quoted: "two",
			level:  store.LevelLine,
			start:  2,
			end:    2,
		},
		{
			name:   "a passage",
			req:    view.CommentRequest{Level: view.CommentLevelPassage, Path: "a.txt", Start: 1, End: 2},
			where:  "a.txt:1–2",
			quoted: "one\ntwo",
			level:  store.LevelPassage,
			start:  1,
			end:    2,
		},
		{
			name:  "a file",
			req:   view.CommentRequest{Level: view.CommentLevelFile, Path: "a.txt"},
			where: "a.txt",
			level: store.LevelFile,
		},
		{
			name:   "a removed line, read from the base",
			req:    view.CommentRequest{Level: view.CommentLevelFile, Path: "a.txt", OldLine: 1, Against: "base"},
			where:  "a.txt",
			quoted: "one",
			level:  store.LevelFile,
		},
		{
			name:  "the review",
			req:   view.CommentRequest{Level: view.CommentLevelReview},
			where: "This review",
			level: store.LevelReview,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			id := actingReview(g, t, f)
			c.req.Round = 1
			c.req.Body = "A point."
			res := sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), c.req)
			stored := readStored(g, res)
			g.Expect(stored.Count).To(BeEquivalentTo(1))
			got := stored.Comment
			g.Expect(got.Where).To(Equal(c.where))
			g.Expect(got.Quoted).To(Equal(c.quoted))
			g.Expect(got.Body).To(Equal("A point."))
			g.Expect(got.Editable).To(BeTrue(), "an open round's comment can still change")
			g.Expect(got.Late).To(BeFalse())
			g.Expect(got.Href).To(Equal("/api/reviews/" + itoa(id) + "/comments/" + strconv.FormatInt(got.ID, 10)))

			held := f.comments(g, t, f.latestRound(g, t, id))
			g.Expect(held).To(HaveLen(1))
			g.Expect(held[0].Level).To(Equal(c.level))
			g.Expect(held[0].StartLine).To(Equal(c.start))
			g.Expect(held[0].EndLine).To(Equal(c.end))

			// The comment is the one the round's view sends for the same store.
			round := readReview(g, f, "/api/reviews/"+itoa(id))
			body := *round.Cards[0].Body
			switch c.req.Level {
			case view.CommentLevelReview:
				g.Expect(round.Acting.Comments).To(Equal([]view.Comment{got}))
			case view.CommentLevelLine:
				g.Expect(body.Threads).To(HaveLen(1))
				g.Expect(body.Threads[0].Comments).To(Equal([]view.Comment{got}))
			case view.CommentLevelFile:
				g.Expect(body.Comments).To(Equal([]view.Comment{got}))
			}
		})
	}
}

// A write that fails in the store answers JSON, which the app reads, and not
// a failure answer.
func TestServer_addCommentJSONFailure(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	g.Expect(f.store.Close()).To(Succeed())
	req := view.CommentRequest{Level: view.CommentLevelReview, Round: 1, Body: "x"}
	res := sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), req)
	g.Expect(res.Code).To(Equal(http.StatusInternalServerError))
	g.Expect(readRefusal(g, res).Message).To(Equal(failedWrite))
}

func TestServer_addCommentJSONLate(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	f.decide(g, t, id, store.VerdictApprove, "")
	nextMillisecond()
	req := view.CommentRequest{Level: view.CommentLevelReview, Round: 1, Body: "One more thing."}
	stored := readStored(g, sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), req))
	g.Expect(stored.Comment.Late).To(BeTrue())
	g.Expect(stored.Comment.Editable).To(BeFalse(), "a released comment cannot change")
}

func TestServer_addCommentJSONMalformed(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		status      int
	}{
		{
			name:        "a form",
			contentType: "application/x-www-form-urlencoded",
			body:        "round=1&level=review&body=x",
			status:      http.StatusUnsupportedMediaType,
		},
		{
			name:        "plain text that reads as JSON",
			contentType: "text/plain",
			body:        `{"round":1,"level":"review","body":"x"}`,
			status:      http.StatusUnsupportedMediaType,
		},
		{
			name:   "no content type",
			body:   `{"round":1,"level":"review","body":"x"}`,
			status: http.StatusUnsupportedMediaType,
		},
		{
			name:        "not JSON",
			contentType: "application/json",
			body:        `round=1`,
			status:      http.StatusBadRequest,
		},
		{
			name:        "a field the type does not hold",
			contentType: "application/json",
			body:        `{"round":1,"level":"review","body":"x","quoted":"forged"}`,
			status:      http.StatusBadRequest,
		},
		{
			name:        "a number as text",
			contentType: "application/json",
			body:        `{"round":"1","level":"review","body":"x"}`,
			status:      http.StatusBadRequest,
		},
		{
			name:        "two bodies",
			contentType: "application/json",
			body:        `{"round":1,"level":"review","body":"x"}{"round":1}`,
			status:      http.StatusBadRequest,
		},
		{
			name:        "a body past the bound",
			contentType: "application/json",
			body:        `{"round":1,"level":"review","body":"` + strings.Repeat("x", maxWriteBytes) + `"}`,
			status:      http.StatusBadRequest,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			id := actingReview(g, t, f)
			res := sendRaw(f.s.Surface(), http.MethodPost, commentsAPI(id), c.contentType, c.body)
			g.Expect(res.Code).To(Equal(c.status))
			g.Expect(readRefusal(g, res).Message).To(Equal(notThisForm))
			comments, err := f.store.Comments(t.Context(), f.latestRound(g, t, id))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(comments).To(BeEmpty())
		})
	}
}

func TestServer_addCommentJSONRefused(t *testing.T) {
	for _, c := range refusedComments {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			id := actingReview(g, t, f)
			if c.req.Round == 0 {
				c.req.Round = 1
			}
			if c.req.Body == "" {
				c.req.Body = "A point."
			}
			res := sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), c.req)
			g.Expect(res.Code).To(Equal(c.status))
			g.Expect(readRefusal(g, res).Message).To(Equal(c.message))
			comments, err := f.store.Comments(t.Context(), f.latestRound(g, t, id))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(comments).To(BeEmpty())
		})
	}

	t.Run("a review that does not exist", func(t *testing.T) {
		g := NewWithT(t)
		f := newFixture(g, t)
		req := view.CommentRequest{Level: view.CommentLevelReview, Round: 1, Body: "x"}
		res := sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(999), req)
		g.Expect(res.Code).To(Equal(http.StatusNotFound))
		g.Expect(readRefusal(g, res).Message).To(Equal(noReview))
	})
}

// A removed row is quoted from the side of the comparison it was removed from:
// the previous round, or with against set to base the base, which can be a
// different text.
func TestServer_addCommentJSONRemovedLine(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	f.decide(g, t, id, store.VerdictChanges, "Rework.")
	review, err := f.store.Review(t.Context(), id)
	g.Expect(err).NotTo(HaveOccurred())
	project, err := f.store.Project(t.Context(), review.ProjectID)
	g.Expect(err).NotTo(HaveOccurred())
	write(g, project.Root, "a.txt", "one\nTWO\n")
	f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: project.Root}, Note: "Again.", Review: id})

	// Round 1 read "one\ntwo\n", and the base read "one\n".
	req := view.CommentRequest{Level: view.CommentLevelFile, Round: 2, Path: "a.txt", OldLine: 2, Body: "Why?"}
	stored := readStored(g, sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), req))
	g.Expect(stored.Comment.Quoted).To(Equal("two"), "the previous round's side")

	req.Against = "base"
	res := sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), req)
	g.Expect(res.Code).To(Equal(http.StatusBadRequest), "the base has no line 2")
	g.Expect(readRefusal(g, res).Message).To(ContainSubstring("has no line 2"))
}

// A retry after a lost answer sends the same key, and the comment is stored
// once, whichever kind of route sent either try.
func TestServer_addCommentJSONSubmission(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	req := view.CommentRequest{Level: view.CommentLevelReview, Round: 1, Body: "Once.", Submission: "key-1"}
	first := readStored(g, sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), req))
	second := readStored(g, sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), req))
	g.Expect(second).To(Equal(first))
	g.Expect(first.Count).To(BeEquivalentTo(1))

	other := req
	other.Submission = "key-2"
	other.Body = "Twice."
	g.Expect(readStored(g, sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), other)).Count).To(BeEquivalentTo(2))
	comments, err := f.store.Comments(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(comments).To(HaveLen(2))
}

// A level the chips chose sends the fields that level takes and no others. The
// requests are the ones the app builds for a line, a passage and a removed row
// switched to the file and to the review, which composerAt.test.ts holds the
// app to, and each is stored as the level it was sent as.
func TestServer_addCommentJSONSwitched(t *testing.T) {
	cases := []struct {
		name   string
		req    view.CommentRequest
		level  store.Level
		path   string
		quoted string
	}{
		{
			name:  "a line sent as the file",
			req:   view.CommentRequest{Level: view.CommentLevelFile, Path: "a.txt"},
			level: store.LevelFile,
			path:  "a.txt",
		},
		{
			name:  "a line sent as the review",
			req:   view.CommentRequest{Level: view.CommentLevelReview},
			level: store.LevelReview,
		},
		{
			name:  "a passage sent as the file",
			req:   view.CommentRequest{Level: view.CommentLevelFile, Path: "a.txt"},
			level: store.LevelFile,
			path:  "a.txt",
		},
		{
			name:  "a passage sent as the review",
			req:   view.CommentRequest{Level: view.CommentLevelReview},
			level: store.LevelReview,
		},
		{
			name:   "a removed row sent as the file, which it is",
			req:    view.CommentRequest{Level: view.CommentLevelFile, Path: "a.txt", OldLine: 1, Against: "base"},
			level:  store.LevelFile,
			path:   "a.txt",
			quoted: "one",
		},
		{
			name:  "a removed row sent as the review",
			req:   view.CommentRequest{Level: view.CommentLevelReview},
			level: store.LevelReview,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := newFixture(g, t)
			id := actingReview(g, t, f)
			c.req.Round = 1
			c.req.Body = "Switched."
			stored := readStored(g, sendJSON(f.s.Surface(), http.MethodPost, commentsAPI(id), c.req))
			g.Expect(stored.Count).To(BeEquivalentTo(1))
			comments, err := f.store.Comments(t.Context(), f.latestRound(g, t, id))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(comments).To(HaveLen(1))
			got := comments[0]
			g.Expect([]any{got.Level, got.Path, got.StartLine, got.EndLine, got.Quoted}).To(Equal([]any{c.level, c.path, 0, 0, c.quoted}))
		})
	}
}

func TestServer_decideJSON(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	path := "/api/reviews/" + itoa(id) + "/decision"
	decide := func(round int32, verdict view.Verdict, note string) *httptest.ResponseRecorder {
		return sendJSON(f.s.Surface(), http.MethodPost, path, view.DecisionRequest{Round: round, Verdict: verdict, Note: note})
	}

	// A change request with nothing in it is refused with its sentence.
	empty := []string{"", "  "}
	for _, note := range empty {
		res := decide(1, view.VerdictChanges, note)
		g.Expect(res.Code).To(Equal(http.StatusUnprocessableEntity))
		g.Expect(readRefusal(g, res).Message).To(Equal(emptyChangeRequest))
	}
	g.Expect(decide(1, "maybe", "").Code).To(Equal(http.StatusBadRequest))
	g.Expect(decide(4, view.VerdictApprove, "").Code).To(Equal(http.StatusNotFound))
	g.Expect(sendRaw(f.s.Surface(), http.MethodPost, path, "text/plain", `{"round":1,"verdict":"approve"}`).Code).To(Equal(http.StatusUnsupportedMediaType))
	_, err := f.store.Decision(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).To(MatchError(store.ErrNotFound), "no refusal stored a verdict")

	res := decide(1, view.VerdictApprove, "Good.")
	g.Expect(res.Code).To(Equal(http.StatusNoContent))
	g.Expect(res.Body.Len()).To(BeZero())
	d, err := f.store.Decision(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(d.Verdict).To(Equal(store.VerdictApprove))
	g.Expect(d.Note).To(Equal("Good."))

	// The same body again names a round decided since.
	res = decide(1, view.VerdictApprove, "")
	g.Expect(res.Code).To(Equal(http.StatusConflict))
	g.Expect(readRefusal(g, res).Message).To(Equal(staleVerdict))
}

func TestServer_decideJSONChangesWithComment(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	f.addComment(g, t, store.NewComment{RoundID: f.latestRound(g, t, id), Level: store.LevelReview, Body: "Fix this."})
	req := view.DecisionRequest{Round: 1, Verdict: view.VerdictChanges}
	res := sendJSON(f.s.Surface(), http.MethodPost, "/api/reviews/"+itoa(id)+"/decision", req)
	g.Expect(res.Code).To(Equal(http.StatusNoContent))
	d, err := f.store.Decision(t.Context(), f.latestRound(g, t, id))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(d.Verdict).To(Equal(store.VerdictChanges))
}

func TestServer_deleteCommentJSON(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	round := f.latestRound(g, t, id)
	c := f.addComment(g, t, store.NewComment{RoundID: round, Level: store.LevelReview, Body: "First."})
	keep := f.addComment(g, t, store.NewComment{RoundID: round, Level: store.LevelReview, Body: "Second."})
	path := "/api/reviews/" + itoa(id) + "/comments/" + itoa(c.ID)
	count := func(res *httptest.ResponseRecorder) int32 {
		g.THelper()
		g.Expect(res.Code).To(Equal(http.StatusOK), res.Body.String())
		var out view.CommentDeleted
		dec := json.NewDecoder(res.Body)
		dec.DisallowUnknownFields()
		g.Expect(dec.Decode(&out)).To(Succeed())
		return out.Count
	}

	other := actingReview(g, t, f)
	res := sendJSON(f.s.Surface(), http.MethodDelete, "/api/reviews/"+itoa(other)+"/comments/"+itoa(c.ID), nil)
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	g.Expect(readRefusal(g, res).Message).To(Equal(noComment))

	g.Expect(count(sendJSON(f.s.Surface(), http.MethodDelete, path, nil))).To(BeEquivalentTo(1))
	g.Expect(count(sendJSON(f.s.Surface(), http.MethodDelete, path, nil))).To(BeEquivalentTo(1), "a comment already gone is gone")

	f.decide(g, t, id, store.VerdictApprove, "")
	res = sendJSON(f.s.Surface(), http.MethodDelete, "/api/reviews/"+itoa(id)+"/comments/"+itoa(keep.ID), nil)
	g.Expect(res.Code).To(Equal(http.StatusConflict))
	g.Expect(readRefusal(g, res).Message).To(Equal(releasedComment))
}

func TestServer_editCommentJSON(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	round := f.latestRound(g, t, id)
	c := f.addComment(g, t, store.NewComment{RoundID: round, Level: store.LevelReview, Body: "First."})
	other := f.addComment(g, t, store.NewComment{RoundID: round, Level: store.LevelReview, Body: "Second."})
	path := "/api/reviews/" + itoa(id) + "/comments/" + itoa(c.ID)
	// held reads each comment's body from the store, which is where an edit
	// that reached too many rows would show.
	held := func() map[int64]string {
		bodies := map[int64]string{}
		for _, got := range f.comments(g, t, round) {
			bodies[got.ID] = got.Body
		}
		return bodies
	}

	stored := readStored(g, sendJSON(f.s.Surface(), http.MethodPut, path, view.CommentEdit{Body: "Edited."}))
	g.Expect(stored.Comment.Body).To(Equal("Edited."))
	g.Expect(stored.Comment.ID).To(Equal(c.ID))
	g.Expect(stored.Count).To(BeEquivalentTo(2))
	g.Expect(held()).To(Equal(map[int64]string{c.ID: "Edited.", other.ID: "Second."}), "the edit changed one comment")

	res := sendJSON(f.s.Surface(), http.MethodPut, path, view.CommentEdit{Body: "  "})
	g.Expect(res.Code).To(Equal(http.StatusBadRequest))
	g.Expect(readRefusal(g, res).Message).To(Equal(emptyComment))

	elsewhere := actingReview(g, t, f)
	res = sendJSON(f.s.Surface(), http.MethodPut, "/api/reviews/"+itoa(elsewhere)+"/comments/"+itoa(c.ID), view.CommentEdit{Body: "Hijacked."})
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	g.Expect(readRefusal(g, res).Message).To(Equal(noComment))

	f.decide(g, t, id, store.VerdictApprove, "")
	res = sendJSON(f.s.Surface(), http.MethodPut, path, view.CommentEdit{Body: "Too late."})
	g.Expect(res.Code).To(Equal(http.StatusConflict))
	g.Expect(readRefusal(g, res).Message).To(Equal(releasedComment))
	g.Expect(held()).To(Equal(map[int64]string{c.ID: "Edited.", other.ID: "Second."}), "a refused edit changed a comment")
}

// A comment already gone is a 404 with a Refusal to an edit, where it was a 500,
// and a delete of one stays a success: the reader wanted it gone, and a delete
// sent twice must not report a failure.
func TestServer_editCommentJSONGone(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	c := f.addComment(g, t, store.NewComment{RoundID: f.latestRound(g, t, id), Level: store.LevelReview, Body: "First."})
	path := "/api/reviews/" + itoa(id) + "/comments/" + itoa(c.ID)
	g.Expect(sendJSON(f.s.Surface(), http.MethodDelete, path, nil).Code).To(Equal(http.StatusOK))

	res := sendJSON(f.s.Surface(), http.MethodPut, path, view.CommentEdit{Body: "Too late."})
	g.Expect(res.Code).To(Equal(http.StatusNotFound))
	g.Expect(readRefusal(g, res).Message).To(Equal(noComment))
	g.Expect(sendJSON(f.s.Surface(), http.MethodDelete, path, nil).Code).To(Equal(http.StatusOK))
}
