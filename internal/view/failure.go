package view

// Failure is what the surface says of a request it could not answer. A page
// load gets it in the shell's eyeball-error tag, which the app draws as an
// error screen. Anything else gets Text as plain text with the status.
type Failure struct {
	// Title is the document's title and the top bar's heading.
	Title string `json:"title"`
	// Heading is what happened, in the box's headline.
	Heading string `json:"heading"`
	// Text is the sentence under it. It is the whole body of the plain answer.
	Text string `json:"text"`
	// WaitingCount is how many reviews wait, which the top bar's eye shows. It
	// is null where the count could not be read, and the eye is then not drawn.
	WaitingCount *int32 `json:"waiting_count"`
}
