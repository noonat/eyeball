package view

// Level is a name with a fixed set of values.
type Level string

const (
	// LevelHigh is the top.
	LevelHigh Level = "high"
	// LevelLow is the bottom.
	LevelLow Level = "low"
)

// Reading is a view holding a level.
type Reading struct {
	// Level is one of the values above.
	Level Level `json:"level"`
}
