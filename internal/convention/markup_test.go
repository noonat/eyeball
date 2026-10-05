package convention

import (
	"testing"

	. "github.com/onsi/gomega"
)

func Test_templateClassesFlagsExactly(t *testing.T) {
	g := NewWithT(t)
	tear, err := TearoutFixture("template-classes")
	g.Expect(err).NotTo(HaveOccurred())

	// The fixture's template uses row, which app.css defines, chrome, which
	// only tearout.css defines, a template action, and undefined. Exactly the
	// middle two are wrong: tearout.css is not shipped, and an action is not a
	// class.
	var what []string
	for _, f := range templateClasses(tear) {
		what = append(what, f.What)
	}
	g.Expect(what).To(ConsistOf(
		"class .chrome is used in a template but app.css does not define it",
		"class .undefined is used in a template but app.css does not define it",
	))
}
