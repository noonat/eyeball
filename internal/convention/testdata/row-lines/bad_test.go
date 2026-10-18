package rowlines

// Each row is flagged or passes by the rule in docs/conventions.md:
// on one line, at most three fields and at most 80 runes from brace to brace;
// otherwise every field on a line of its own, with both braces alone.
var rows = []struct {
	a, b, c, d int
	name, want string
}{
	{a: 1, b: 2, c: 3, d: 4},
	{name: "a name long enough to matter", want: "a want that pushes the row past eighty"},
	{
		name: "two fields", want: "on one line",
	},
	{
		name: "the closing brace",
		want: "shares the last line"},
	{a: 1, b: 2, c: 3},
	{name: "ünïcödé ünïcödé ünïcödé ünïcödé", want: "ünïcödé ünïcödé ünïcödé ü"},
	{
		name: "stacked",
		want: "as the rule asks",
	},
}
