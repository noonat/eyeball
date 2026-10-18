package convention

import (
	"go/token"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_rowLinesFlagsEachCase(t *testing.T) {
	g := NewWithT(t)
	fset := token.NewFileSet()
	pkg, err := ParseFixture(fset, "row-lines")
	g.Expect(err).NotTo(HaveOccurred())

	// Line 19 is 76 runes and 105 bytes, so it passes only when the width is
	// counted in runes. bad.go's five-field row passes because it is not in a
	// test.
	var found []string
	for _, f := range rowLines(fset, pkg.Files) {
		found = append(found, f.At+" "+f.What)
	}
	dir := "internal/convention/testdata/row-lines/bad_test.go:"
	g.Expect(found).To(ConsistOf(
		dir+"10 table row holds 4 fields on one line; put each on its own line",
		dir+"11 table row is 86 runes wide on one line; put each field on its own line",
		dir+"13 table row field shares a line; put each field on its own line",
		dir+"17 table row's closing brace shares a line with its last field",
	))
}
