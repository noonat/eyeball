package view

// Queue is the view of the screen the surface opens on.
type Queue struct {
	// Waiting is every review with a request outstanding, longest wait first.
	// A section with no rows is sent as null.
	Waiting []Row `json:"waiting"`
	// Working is the reviews an agent holds, most recently answered first.
	Working []Row `json:"working"`
	// Settled is what was approved recently, newest first and cut short.
	Settled []Row `json:"settled"`
	// WaitingCount is how many reviews wait, which the top bar's eye shows.
	WaitingCount int32 `json:"waiting_count"`
	// Stamp is computed from the rows above. The live stream sends the same
	// stamp for the queue, and the app refetches when the two differ.
	Stamp string `json:"stamp"`
}

// Row is one review as a queue row shows it.
type Row struct {
	// ID is the review's id, which its link is built from. It is sent as a
	// string so it never passes through a double.
	ID int64 `json:"id,string"`
	// Project is where the review came from. It is empty on the project's own
	// page, and the row leaves it out.
	Project string `json:"project"`
	// Agent is the agent that opened the review.
	Agent string `json:"agent"`
	// Title is what the row is called.
	Title string `json:"title"`
	// Age is how long the row has been in its section: since the round was
	// asked for while it waits, and since the verdict once there is one.
	Age string `json:"age"`
	// Round is the latest round's number.
	Round int32 `json:"round"`
	// Files is the count of files that moved, written out. A waiting row is the
	// only one that prints it.
	Files string `json:"files"`
	// Added is the lines added, written out with separators.
	Added string `json:"added"`
	// Removed is the lines removed, written out with separators.
	Removed string `json:"removed"`
	// Gist is the first paragraph of the latest round's note, which a waiting
	// row quotes under its title.
	Gist string `json:"gist"`
}
