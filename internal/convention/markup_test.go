package convention

import (
	"testing"

	. "github.com/onsi/gomega"
)

func Test_componentClassesFlagsExactly(t *testing.T) {
	g := NewWithT(t)
	tear, err := TearoutFixture("component-classes")
	g.Expect(err).NotTo(HaveOccurred())

	// chrome is defined by tearout.css, which a story may use and a component
	// may not. A name built from ${tone} is not read, and the literal words
	// around it are.
	var what []string
	for _, f := range componentClasses(tear) {
		what = append(what, f.At+" "+f.What)
	}
	g.Expect(what).To(ConsistOf(
		"testdata/component-classes/components/Bad.stories.tsx:1 class .also-undefined is used in a component but no stylesheet it may use defines it",
		"testdata/component-classes/components/Bad.tsx:3 class .chrome is used in a component but no stylesheet it may use defines it",
		"testdata/component-classes/components/Bad.tsx:4 class .missing-in-template is used in a component but no stylesheet it may use defines it",
		"testdata/component-classes/components/Bad.tsx:5 class .missing-in-cx is used in a component but no stylesheet it may use defines it",
		"testdata/component-classes/components/Bad.tsx:6 class .undefined-anywhere is used in a component but no stylesheet it may use defines it",
	))
}

func Test_tearoutClassesReadsEveryStylesheet(t *testing.T) {
	g := NewWithT(t)
	tear, err := TearoutFixture("tearout-classes")
	g.Expect(err).NotTo(HaveOccurred())

	// third is defined only in direction.css, which a tearout trying out a
	// direction links beside app.css, so the page may use it.
	var what []string
	for _, f := range tearoutClasses(tear) {
		what = append(what, f.What)
	}
	g.Expect(what).To(ConsistOf(
		"class .undefined-by-any-stylesheet is used but no stylesheet defines it",
	))
}

func Test_templateClassesFlagsExactly(t *testing.T) {
	g := NewWithT(t)
	tear, err := TearoutFixture("template-classes")
	g.Expect(err).NotTo(HaveOccurred())

	// The fixture's template uses row, which app.css defines, chrome, which
	// only tearout.css defines, third, which only a direction's stylesheet
	// defines, a template action, and undefined. Every class but row is wrong,
	// because only app.css is shipped, and an action is not a class.
	var what []string
	for _, f := range templateClasses(tear) {
		what = append(what, f.What)
	}
	g.Expect(what).To(ConsistOf(
		"class .chrome is used in a template but app.css does not define it",
		"class .third is used in a template but app.css does not define it",
		"class .undefined is used in a template but app.css does not define it",
	))
}
