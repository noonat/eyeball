package bad

// wrapped breaks its parameter list across lines instead of taking fewer
// parameters, which the check reports.
func wrapped(
	a int,
	b string,
) int {
	return a + len(b)
}
