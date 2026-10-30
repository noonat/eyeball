package view

// Count is a view with an integer that can pass 2^53.
type Count struct {
	// N is too wide for a double.
	N int64 `json:"n"`
}
