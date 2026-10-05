package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func Test_seedAnswersEverythingWaiting(t *testing.T) {
	g := NewWithT(t)
	s := seeded(g, t)
	g.Expect(s.answerAll()).To(Succeed())
	q, err := s.store.Queue(context.Background(), 5)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(q.Waiting).To(BeEmpty())
	g.Expect(q.Working).To(HaveLen(5))
}

func Test_seedRefusesADirectoryThatExists(t *testing.T) {
	g := NewWithT(t)
	dir, err := os.MkdirTemp("", "ebs-")
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	_, err = newSeed(context.Background(), dir)
	g.Expect(err).To(MatchError(ContainSubstring("exists")))
	entries, err := os.ReadDir(dir)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(entries).To(BeEmpty(), "the seed wrote into a directory it refused")
}

func Test_seedWritesEveryQueueSection(t *testing.T) {
	g := NewWithT(t)
	s := seeded(g, t)
	q, err := s.store.Queue(context.Background(), 5)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(q.Waiting).To(HaveLen(3))
	g.Expect(q.Working).To(HaveLen(2))
	g.Expect(q.Settled).To(HaveLen(2))
	// Backdated as the tearouts' ages read: the longest wait is three hours.
	g.Expect(s.now.Sub(q.Waiting[0].RequestedAt)).To(BeNumerically("~", 3*time.Hour, time.Millisecond), "the store keeps milliseconds")
	g.Expect(q.Waiting[2].Round).To(Equal(2), "the product document waits on its second round")
}

// seeded is a seed written into a fresh directory with a short path, since
// the state directory's socket path has a length limit t.TempDir passes.
func seeded(g *WithT, t *testing.T) *seed {
	g.THelper()
	parent, err := os.MkdirTemp("", "ebs-")
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = os.RemoveAll(parent) })
	s, err := newSeed(context.Background(), filepath.Join(parent, "s"))
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = s.store.Close() })
	g.Expect(s.write()).To(Succeed())
	return s
}
