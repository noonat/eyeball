package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/view"
)

func TestGenerate_committedFilesAreCurrent(t *testing.T) {
	g := NewWithT(t)
	files, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(files).To(HaveLen(2 * len(states)))
	for name, body := range files {
		have, err := os.ReadFile(filepath.Join("../..", outDir, name))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(have)).To(Equal(string(body)), "run: make api")
	}
}

func TestGenerate_everyRunIsTheSame(t *testing.T) {
	g := NewWithT(t)
	first, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	second, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second).To(Equal(first))
}

func TestGenerate_statesHoldTheirSections(t *testing.T) {
	g := NewWithT(t)
	files, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	read := func(name string) view.Queue {
		var q view.Queue
		g.Expect(json.Unmarshal(files[name], &q)).To(Succeed())
		return q
	}
	populated := read("queue-populated.json")
	g.Expect(populated.Waiting).To(HaveLen(3))
	g.Expect(populated.Working).To(HaveLen(2))
	g.Expect(populated.Settled).To(HaveLen(2))
	g.Expect(populated.WaitingCount).To(Equal(int32(3)))

	empty := read("queue-nothing-waiting.json")
	g.Expect(empty.Waiting).To(BeNil())
	g.Expect(empty.WaitingCount).To(BeZero())
	g.Expect(string(files["queue-nothing-waiting.json"])).To(ContainSubstring(`"waiting": null`))

	awkward := read("queue-awkward.json")
	g.Expect(awkward.Waiting).To(HaveLen(8))
	g.Expect(awkward.Waiting[0].Files).To(Equal("11 files"))
	g.Expect(awkward.Waiting[0].Age).To(Equal("4d"))
}
