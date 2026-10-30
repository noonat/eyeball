package kinds

import "time"

// Mode is how a thing is shown.
type Mode string

const (
	// ModeDark is the dark theme.
	ModeDark Mode = "dark"
	// ModeLight is the light theme.
	ModeLight Mode = "light"
)

// Item is a thing with one of each kind.
type Item struct {
	// ID is sent as a string.
	ID int64 `json:"id,string"`
	// Name is plain.
	Name string
	// Mode is a union.
	Mode Mode `json:"mode"`
	// At is a time.
	At time.Time `json:"at"`
	// Tags is a slice, so it can be null.
	Tags []string `json:"tags"`
	// Scores is a map, so it can be null.
	Scores map[string]int32 `json:"scores"`
	// Next is a pointer, so it can be null.
	Next *Item `json:"next"`
	// Data is bytes, which JSON writes as base64.
	Data []byte `json:"data"`
	// Note may be left out.
	Note string `json:"note,omitempty"`
	// Grid is a slice of slices.
	Grid [][]float64 `json:"grid"`
	// Holes is a slice that holds pointers.
	Holes []*Item `json:"holes"`
	// Skipped is not sent.
	Skipped string `json:"-"`
	private string
}
