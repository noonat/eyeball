package main

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestGenerate_committedFileIsCurrent(t *testing.T) {
	g := NewWithT(t)
	got, err := Generate("../../" + sourceDir)
	g.Expect(err).NotTo(HaveOccurred())
	have, err := os.ReadFile("../../" + outFile)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(have)).To(Equal(string(got)), "run: make api")
}

func TestGenerate_everyKindMapsExactly(t *testing.T) {
	g := NewWithT(t)
	got, err := Generate("testdata/kinds")
	g.Expect(err).NotTo(HaveOccurred())
	text := string(got)
	g.Expect(text).To(ContainSubstring("  id: string;\n"), "a quoted int64 is a string")
	g.Expect(text).To(ContainSubstring("  Name: string;\n"), "a field with no tag keeps its Go name")
	g.Expect(text).To(ContainSubstring("  mode: Mode;\n"))
	g.Expect(text).To(ContainSubstring("  at: string;\n"))
	g.Expect(text).To(ContainSubstring("  tags: string[] | null;\n"), "a nil slice is null")
	g.Expect(text).To(ContainSubstring("  scores: Record<string, number> | null;\n"), "a nil map is null")
	g.Expect(text).To(ContainSubstring("  next: Item | null;\n"), "a nil pointer is null")
	g.Expect(text).To(ContainSubstring("  data: string | null;\n"))
	g.Expect(text).To(ContainSubstring("  note?: string;\n"), "omitempty is optional")
	g.Expect(text).To(ContainSubstring("  grid: (number[] | null)[] | null;\n"))
	g.Expect(text).To(ContainSubstring("  holes: (Item | null)[] | null;\n"))
	g.Expect(text).NotTo(ContainSubstring("Skipped"))
	g.Expect(text).NotTo(ContainSubstring("private"))
	g.Expect(text).To(ContainSubstring("export type Mode = \"dark\" | \"light\";\n"))
}

func TestGenerate_interfaceFails(t *testing.T) {
	g := NewWithT(t)
	_, err := Generate("testdata/interface")
	g.Expect(err).To(MatchError(ContainSubstring("Reply.Extra: an interface has no exact form")))
}

func TestGenerate_mapWithoutStringKeysFails(t *testing.T) {
	g := NewWithT(t)
	_, err := Generate("testdata/intkey")
	g.Expect(err).To(MatchError(ContainSubstring("Table.Names: a map keyed by int")))
}

func TestGenerate_unionFollowsItsConstants(t *testing.T) {
	g := NewWithT(t)
	got, err := Generate("testdata/union")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(got)).To(ContainSubstring("export type Level = \"high\" | \"low\";\n"))

	// The same package with one more constant, planted in a copy.
	dir := t.TempDir()
	src, err := os.ReadFile("testdata/union/union.go")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(os.WriteFile(filepath.Join(dir, "union.go"), src, 0o644)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(dir, "more.go"), []byte("package view\n\n// LevelMid is between.\nconst LevelMid Level = \"mid\"\n"), 0o644)).To(Succeed())
	planted, err := Generate(dir)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(planted)).NotTo(Equal(string(got)))
	g.Expect(string(planted)).To(ContainSubstring("export type Level = \"high\" | \"low\" | \"mid\";\n"))
}

func TestGenerate_wideIntegersFail(t *testing.T) {
	g := NewWithT(t)
	_, err := Generate("testdata/wide")
	g.Expect(err).To(MatchError(ContainSubstring("Count.N: int64 can pass 2^53")))
	// int is as wide as the machine, so it is refused the same way.
	_, err = Generate("testdata/plainint")
	g.Expect(err).To(MatchError(ContainSubstring("Small.N: int can pass 2^53")))
}
