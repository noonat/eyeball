package view

// Review is the view of one round of a review: the brief, the verdict, the
// cards for the files that moved, and what the reader can do.
type Review struct {
	// Title is the document's title and the top bar's heading.
	Title string `json:"title"`
	// Back is where the top bar's back link goes.
	Back string `json:"back"`
	// BackLabel is the back link's text.
	BackLabel string `json:"back_label"`
	// WaitingCount is how many reviews wait, which the top bar's eye shows. It
	// is null where the count could not be read.
	WaitingCount *int32 `json:"waiting_count"`
	// ID is the review's id, sent as a string so it never passes through a
	// double.
	ID int64 `json:"id,string"`
	// Round is the round's number.
	Round int32 `json:"round"`
	// ReviewTitle is the title the review was opened with. Title above is the
	// round's, which an earlier round's page names differently.
	ReviewTitle string `json:"review_title"`
	// Project is the project's name.
	Project string `json:"project"`
	// ProjectHref is the project's view, which the brief's project links to.
	ProjectHref string `json:"project_href"`
	// Agent is the agent that opened the review.
	Agent string `json:"agent"`
	// Abandoned says the agent withdrew the review, which no round's verdict
	// records.
	Abandoned bool `json:"abandoned"`
	// Waiting is how long the round has been asked for.
	Waiting string `json:"waiting"`
	// Decided is the verdict, and is null while the round waits.
	Decided *Decided `json:"decided"`
	// Note is the agent's brief, a paragraph to an entry.
	Note []string `json:"note"`
	// Files is the count of files shown, written out, and is empty where nothing
	// could be compared. Added and Removed are the lines each way.
	Files string `json:"files"`
	// Added is the lines added.
	Added string `json:"added"`
	// Removed is the lines removed.
	Removed string `json:"removed"`
	// Against is the control choosing what the change is measured against, and
	// is null on a first round, which has only its base.
	Against *Against `json:"against"`
	// Cards is every in-scope file that moved, in path order.
	Cards []Card `json:"cards"`
	// Listed says a list of the files, linking to their cards, comes first.
	Listed bool `json:"listed"`
	// Fresh says the one card is a new document, which the page draws as the
	// document with no card around it.
	Fresh bool `json:"fresh"`
	// Comments are the comments that belong to no card shown: the review's
	// own on a round that cannot be commented on, and any on a file this
	// comparison does not show.
	Comments []Comment `json:"comments"`
	// Replies are the comments the agent has answered, each with its answer.
	Replies []Comment `json:"replies"`
	// Earlier lists the rounds before this one, on the current round only.
	Earlier []EarlierRound `json:"earlier"`
	// Closed says an earlier round is closed and which round is current, and
	// is empty on the current round.
	Closed string `json:"closed"`
	// AllFiles is the tree's root at this round, keeping the comparison.
	AllFiles string `json:"all_files"`
	// Acting is what the reader can do, and is null on a round nothing more
	// can be said on.
	Acting *Acting `json:"acting"`
	// Stream is the live stream this view opens.
	Stream string `json:"stream"`
	// Stamp is the stamp the stream sends for this review. The app refetches
	// when the two differ.
	Stamp string `json:"stamp"`
	// Place keys where a reader was on this view: the review, the round and the
	// comparison.
	Place string `json:"place"`
	// Unmeasurable says why nothing could be compared, where git can no longer
	// read the round's base, and is empty otherwise.
	Unmeasurable string `json:"unmeasurable"`
}

// Decided is a round's verdict as the page shows it.
type Decided struct {
	// Approved says which verdict it was.
	Approved bool `json:"approved"`
	// Pill is the brief's state pill: the verdict and how long ago it went out.
	Pill string `json:"pill"`
	// Sentence says what went out with it and when.
	Sentence string `json:"sentence"`
	// Note is the verdict note, a paragraph to an entry.
	Note []string `json:"note"`
}

// Against is the control choosing what the change is measured against.
type Against struct {
	// Previous is the previous-round link's label.
	Previous string `json:"previous"`
	// Base says the whole review is shown rather than the latest change.
	Base bool `json:"base"`
}

// EarlierRound is one row of the list of earlier rounds.
type EarlierRound struct {
	// Href is the round's page.
	Href string `json:"href"`
	// Label names the round and its verdict.
	Label string `json:"label"`
	// Age is how long ago it was decided.
	Age string `json:"age"`
}

// Acting is what the reader can do on the current round: comment and decide
// while it is open, or add a late comment once it is decided.
type Acting struct {
	// State is "open" on an undecided round and "late" on a decided one.
	State string `json:"state"`
	// Against is the comparison, "base" or empty, which a removed row's quote
	// needs.
	Against string `json:"against"`
	// ListID is the element id of the review's own list of comments.
	ListID string `json:"list_id"`
	// Comments are the review-level comments.
	Comments []Comment `json:"comments"`
	// Bar is the decision bar, and is null once the round is decided.
	Bar *Bar `json:"bar"`
}

// Bar is the decision bar on an open current round.
type Bar struct {
	// Action is the address the verdict posts to.
	Action string `json:"action"`
	// Count is the comments the round holds.
	Count int32 `json:"count"`
}

// Card is one file of a round.
type Card struct {
	// Path is the file's path.
	Path string `json:"path"`
	// ID is the card's element id, which the list of files links to.
	ID string `json:"id"`
	// Added and Removed are the file's lines each way, written out.
	Added string `json:"added"`
	// Removed is the lines removed.
	Removed string `json:"removed"`
	// Unmeasured says the file has no counts to show.
	Unmeasured bool `json:"unmeasured"`
	// Whole is the file's own page, keeping the comparison.
	Whole string `json:"whole"`
	// RawDiff is a document's Source view, and is empty for any other file.
	RawDiff string `json:"raw_diff"`
	// Held is the lines the file moved, written out, where the card shows a
	// line to load it anyway in place of its body, and is empty otherwise.
	Held string `json:"held"`
	// HeldHint says why a held card shows no body.
	HeldHint string `json:"held_hint"`
	// Deferred says the body was not sent with the round and is asked for at
	// BodyHref.
	Deferred bool `json:"deferred"`
	// BodyHref is the card's body on its own.
	BodyHref string `json:"body_href"`
	// SkeletonHunks and SkeletonLines size the skeleton a card shows until its
	// body arrives, and are zero for a body that is not rows.
	SkeletonHunks int32 `json:"skeleton_hunks"`
	// SkeletonLines is how many lines the body will hold.
	SkeletonLines int32 `json:"skeleton_lines"`
	// Body is the card's body, and is null where it is deferred or held.
	Body *CardBody `json:"body"`
}

// CardBody is what a card shows for one file, with the comments on it.
type CardBody struct {
	// Kind says which of the fields below holds what the card shows.
	Kind FileBody `json:"kind"`
	// Mode is the line saying the file's mode changed, and is empty otherwise.
	Mode string `json:"mode"`
	// Rendered is the file shown as itself, and is null unless Kind is
	// rendered.
	Rendered *Rendering `json:"rendered"`
	// Passages are a document's passages, and are null unless Kind is
	// document.
	Passages []DocPart `json:"passages"`
	// Rows are the file's rows of code, and are null unless Kind is rows.
	Rows []CodeRow `json:"rows"`
	// Threads are the comments on a line, each shown under its row.
	Threads []Thread `json:"threads"`
	// ListID is the element id of the file's list of comments, and is empty
	// where the round takes no comments.
	ListID string `json:"list_id"`
	// Comments are the comments on the file as a whole, and on a line the rows
	// do not show.
	Comments []Comment `json:"comments"`
	// Slot is the element id of the slot a composer fills for a tapped line,
	// and is empty where no composer opens.
	Slot string `json:"slot"`
}

// Thread is the comments on one line or one passage.
type Thread struct {
	// ID is the thread's element id.
	ID string `json:"id"`
	// Line is the new side's line number a line thread is under, and is zero
	// for a passage's.
	Line int32 `json:"line"`
	// Comments are the thread's comments.
	Comments []Comment `json:"comments"`
}

// Comment is one comment, or one of an agent's replies. Its text is a string
// the app escapes.
type Comment struct {
	// ID is the comment's id, sent as a string.
	ID int64 `json:"id,string"`
	// Href is the comment's own address, which Edit and Delete act on.
	Href string `json:"href"`
	// Where is what the comment is attached to.
	Where string `json:"where"`
	// Quoted is the text it points at, and is empty where it points at none.
	Quoted string `json:"quoted"`
	// Body is what the reviewer wrote, or the agent's answer in a list of
	// replies.
	Body string `json:"body"`
	// Late says it was written after the verdict, or on a round that can no
	// longer be decided.
	Late bool `json:"late"`
	// Editable says its round has not been released, so it can change.
	Editable bool `json:"editable"`
}
