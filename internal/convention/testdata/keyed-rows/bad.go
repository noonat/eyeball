package keyedrows

// Rows is a table whose fields are positional.
var Rows = []struct {
	name string
	path string
	want bool
}{
	{"markdown", "a.md", true},
	{"binary", "a.png", false},
}
