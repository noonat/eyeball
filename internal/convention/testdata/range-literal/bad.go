package rangeliteral

// Walk ranges over a literal written in place.
func Walk() int {
	n := 0
	for range []string{"a.md", "b.go", "c.png"} {
		n++
	}
	return n
}
