package convention

import (
	"testing"

	. "github.com/onsi/gomega"
)

func Test_cssDeclarationsReadsAppCSSAlone(t *testing.T) {
	g := NewWithT(t)
	tear, err := TearoutFixture("css-declarations")
	g.Expect(err).NotTo(HaveOccurred())

	// direction.css declares --bg and --pad, which the fixture's app.css lacks.
	// A direction's stylesheet redeclares tokens as a matter of course, so it
	// must not hide a declaration app.css has lost.
	var what []string
	for _, f := range cssDeclarations(tear) {
		what = append(what, f.What)
	}
	g.Expect(what).To(ContainElements("no longer declares --bg:", "no longer declares --pad:"))
	g.Expect(what).NotTo(ContainElement("no longer declares --ink:"))
}

func Test_iconsListedReadsAppCSSAlone(t *testing.T) {
	g := NewWithT(t)
	tear, err := TearoutFixture("icons-listed")
	g.Expect(err).NotTo(HaveOccurred())

	// direction.css maps pencil, which icons.txt does not list. The list and
	// the table it generates are both app.css's, so another stylesheet's mask
	// is not part of the comparison.
	var what []string
	for _, f := range iconsListed(tear) {
		what = append(what, f.What)
	}
	g.Expect(what).To(ConsistOf(
		"lists ghost-only-in-the-list but app.css has no mask for it",
		"maps mapped-but-unlisted but icons.txt does not list it",
	))
}
