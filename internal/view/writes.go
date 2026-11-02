package view

// CommentRequest is the body of POST /api/reviews/{review}/comments: one
// comment, with the anchor the reader tapped. The round, the anchor and the
// comparison say what the reader was looking at.
type CommentRequest struct {
	// Round is the number of the round the reader was looking at, so a comment
	// is never stored on a round the reader did not see.
	Round int32 `json:"round"`
	// Level says what the comment attaches to.
	Level CommentLevel `json:"level"`
	// Path is the file, and is empty for the review's own comment.
	Path string `json:"path"`
	// Start and End are a line's or a passage's range. Zero is absent, and End
	// is absent for a single line.
	Start int32 `json:"start"`
	// End is the last line of a passage.
	End int32 `json:"end"`
	// OldLine is a removed row's number on the side it was removed from, and
	// zero where the comment is not on a removed row.
	OldLine int32 `json:"old_line"`
	// Against is the comparison the reader was in, "base" or empty, which says
	// which side a removed row was read from.
	Against string `json:"against"`
	// Body is the comment's text.
	Body string `json:"body"`
	// Submission is the key the app drew when it first sent the comment. A retry
	// sends the same key, so a first try that reached the store and lost its
	// answer is stored once.
	Submission string `json:"submission"`
}

// CommentLevel is what a comment attaches to.
type CommentLevel string

// The levels a comment can be written at.
const (
	// CommentLevelReview is a comment on the whole review.
	CommentLevelReview CommentLevel = "review"
	// CommentLevelFile is a comment on one file, or on one of its removed
	// rows.
	CommentLevelFile CommentLevel = "file"
	// CommentLevelLine is a comment on one line of a file.
	CommentLevelLine CommentLevel = "line"
	// CommentLevelPassage is a comment on a range of lines, or a passage of a
	// document.
	CommentLevelPassage CommentLevel = "passage"
)

// CommentEdit is the body of PUT /api/reviews/{review}/comments/{comment}.
type CommentEdit struct {
	// Body is the comment's new text.
	Body string `json:"body"`
}

// CommentStored is the answer to a comment that was added or edited: the
// comment as the round's view sends it, and the count of comments the review's
// latest round holds, which the bar shows.
type CommentStored struct {
	// Comment is the stored comment.
	Comment Comment `json:"comment"`
	// Count is the comments on the review's latest round.
	Count int32 `json:"count"`
}

// CommentDeleted is the answer to a comment that was deleted, or was already
// gone.
type CommentDeleted struct {
	// Count is the comments on the review's latest round.
	Count int32 `json:"count"`
}

// DecisionRequest is the body of POST /api/reviews/{review}/decision.
type DecisionRequest struct {
	// Round is the number of the round the reader was looking at, which must
	// still be the open review's latest.
	Round int32 `json:"round"`
	// Verdict is the decision.
	Verdict Verdict `json:"verdict"`
	// Note is the text sent with the verdict, and is empty for none.
	Note string `json:"note"`
}

// Verdict is a decision on a round.
type Verdict string

// The decisions a reader can send.
const (
	// VerdictApprove releases the round as approved.
	VerdictApprove Verdict = "approve"
	// VerdictChanges asks the agent for changes, and needs a comment or a note.
	VerdictChanges Verdict = "changes"
)

// Refusal is what a write answers with when the server will not do it, with a
// 4xx status. The app shows the sentence beside the control that sent the
// request, and leaves the text where it was typed.
type Refusal struct {
	// Message is one sentence saying what happened in the reader's terms.
	Message string `json:"message"`
}
