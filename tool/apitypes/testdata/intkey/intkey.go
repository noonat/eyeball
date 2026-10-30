package view

// Table is a view keyed by something JSON cannot key by.
type Table struct {
	// Names is keyed by an integer.
	Names map[int]string `json:"names"`
}
