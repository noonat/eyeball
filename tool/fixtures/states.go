package main

import (
	"time"

	"github.com/noonat/eyeball/internal/store"
)

// state is one queue the app can be drawn in, and the name its files share.
type state struct {
	file    string
	reviews []review
}

// review is one review's history, ending in the state the queue shows.
type review struct {
	project, agent, title, note string
	// round is the latest round's number, and files, added and removed what
	// it moved.
	round, files, added, removed int
	// ago is how long ago the latest round was asked for, or answered when
	// verdict is not empty.
	ago     time.Duration
	verdict store.Verdict
}

// The content is the queue.html tearout's, so a story draws what the phone
// approved. A working row's "Not requested yet" frame has no state here: a row
// is Working only once a round has an answer.
var states = []state{
	{file: "queue-populated", reviews: populated},
	{file: "queue-nothing-waiting", reviews: nothingWaiting},
	{file: "queue-awkward", reviews: awkward},
}

var settledReviews = []review{
	{
		project: "harbor", agent: "fix/outbox-drain", title: "Delta sync watermark",
		note:  "The watermark never moves backward.",
		round: 2, files: 1, added: 12, removed: 3, ago: 2 * time.Hour, verdict: store.VerdictApprove,
	},
	{
		project: "eyeball", agent: "spec/012-surface", title: "Voice, both kinds",
		note:  "The prose voice and the conversational one, written down.",
		round: 1, files: 1, added: 40, removed: 0, ago: 30 * time.Hour, verdict: store.VerdictApprove,
	},
}

var workingReviews = []review{
	{
		project: "eyeball", agent: "spec/014-queue", title: "Queue screen markup",
		note:  "Markup for the queue.",
		round: 2, files: 2, added: 90, removed: 14, ago: 4 * time.Minute, verdict: store.VerdictChanges,
	},
	{
		project: "harbor", agent: "fix/outbox-drain", title: "Offline outbox drainer",
		note:  "Drains what queued while offline.",
		round: 1, files: 1, added: 31, removed: 0, ago: 26 * time.Minute, verdict: store.VerdictChanges,
	},
}

var populated = concat(
	[]review{
		{
			project: "eyeball", agent: "spec/012-surface", title: "Product doc, second pass",
			note:  "Both comments from round 1 are done. The widening section now names the control instead of describing the behavior, and the base-is-empty case moved out of Open questions into A round is frozen.",
			round: 2, files: 1, added: 214, removed: 38, ago: 12 * time.Minute,
		},
		{
			project: "harbor", agent: "feat/upload-retry", title: "Upload worker and its retry ladder",
			note:  "The worker retries three times with backoff, then gives up and records the failure on the job. Measured against the staging bucket: a cold connection takes 40s, so the first wait is 60s not 5s.",
			round: 1, files: 6, added: 341, removed: 52, ago: time.Hour,
		},
		{
			project: "field-notes", agent: "spec/018-tag-import", title: "Spec 018: bulk import for the tag index",
			note:  "The importer now refuses a file with duplicate tags instead of merging them.",
			round: 3, files: 2, added: 58, removed: 11, ago: 3 * time.Hour,
		},
	},
	workingReviews,
	settledReviews,
)

var nothingWaiting = concat(workingReviews, settledReviews)

// The week that breaks the layout: titles that wrap to three lines, a
// path-shaped agent name, a review waiting since Tuesday, an eleven-file
// change, and eight waiting at once.
var awkward = []review{
	{
		project: "field-notes", agent: "spec/026-index-migration",
		title: "Spec 026: move the tag index out of the manifest and into its own table, with a migration that survives a half-written state",
		note:  "The largest change in the series. The manifest stops holding tags, a table of its own holds them, and the migration writes a marker row first, so a run stopped halfway can tell which half it finished and resume from there.",
		round: 5, files: 11, added: 1284, removed: 903, ago: 4*24*time.Hour + 2*time.Hour,
	},
	{
		project: "harbor", agent: "feat/upload-retry", title: "sha256 short ids collide at about 40k rows and nothing handles it",
		note:  "Two ids already collide in the test fixture at 41,203 rows. This round adds the test that shows it, not a fix.",
		round: 1, files: 1, added: 6, removed: 2, ago: 2*24*time.Hour + time.Hour,
	},
	{
		project: "eyeball", agent: "spec/012-surface", title: ".claude/voice.local.md override, and the hook that prefers it",
		note:  "The hook reads voice.local.md first and falls back to voice.md.",
		round: 2, files: 3, added: 41, removed: 9, ago: 7 * time.Hour,
	},
	{
		project: "harbor", agent: "fix/write-lock", title: "busy_timeout on the write path",
		note:  "Sets busy_timeout on the write connection, so a second writer waits instead of failing.",
		round: 1, files: 2, added: 18, removed: 4, ago: 6 * time.Hour,
	},
	{
		project: "eyeball", agent: "spec/014-queue", title: "Diff table markup",
		note:  "Round 4 moves the word marks into spans of their own and drops the table's fixed layout.",
		round: 4, files: 2, added: 77, removed: 140, ago: 5 * time.Hour,
	},
	{
		project: "harbor", agent: "/home/dev/src/harbor/.claude/worktrees/feat-upload-retry-with-a-long-branch-name",
		title: "Retry",
		note:  "A title of one word and an agent name that is a whole path, so the agent line has to truncate.",
		round: 1, files: 1, added: 3, removed: 0, ago: 4 * time.Hour,
	},
	{
		project: "field-notes", agent: "spec/018-tag-import", title: "Import",
		note:  "Renames the importer.",
		round: 1, files: 1, added: 1, removed: 0, ago: 90 * time.Minute,
	},
	{
		project: "eyeball", agent: "spec/030-a-name-long-enough-to-need-the-ellipsis", title: "The changelog entry for every spec merged since the last release, grouped by area and written for a reader who has not seen the specs",
		note:  "A paragraph that wraps well past three lines on a phone, to show where the gist stops. It keeps going: the second sentence adds a fact, the third repeats it, and the fourth is here so the paragraph is long enough to show how the row holds a gist this size.",
		round: 1, files: 4, added: 66, removed: 12, ago: 20 * time.Minute,
	},
}

// concat joins lists of reviews in order.
func concat(lists ...[]review) []review {
	var out []review
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}
