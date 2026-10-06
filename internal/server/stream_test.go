package server

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/store"
)

// A review's stream starts with the stamp its page printed, moves on a reply
// or a verdict, and stays put on the reader's own comment.
func TestServer_handleEvents(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)

	printed := attr(find(f.page(g, "/reviews/"+itoa(id), http.StatusOK), "page"), "data-stamp")
	g.Expect(printed).NotTo(BeEmpty())
	events := openStream(g, t, srv.URL+"/events?review="+itoa(id))
	g.Eventually(events).Should(Receive(Equal("retry: 2000")))
	g.Eventually(events).Should(Receive(Equal("data: " + printed)))

	c := f.comment(g, t, id, "A point.")
	g.Consistently(events, 300*time.Millisecond).ShouldNot(Receive(HavePrefix("data:")), "the reader's own comment moved the stamp")

	f.decide(g, t, id, store.VerdictChanges, "")
	var moved string
	g.Eventually(events).Should(Receive(&moved))
	g.Expect(moved).To(HavePrefix("data: "))
	g.Expect(moved).NotTo(Equal("data: " + printed))

	g.Expect(f.store.Reply(t.Context(), c.ID, "Done.")).To(Succeed())
	f.s.hub.publish()
	var replied string
	g.Eventually(events).Should(Receive(&replied))
	g.Expect(replied).NotTo(Equal(moved), "a reply moves the stamp")
}

// A page with no stamp holds a stream for its connection alone, and nothing
// published sends it one.
func TestServer_handleEventsPlain(t *testing.T) {
	g := NewWithT(t)
	f := newFixture(g, t)
	id := actingReview(g, t, f)
	srv := httptest.NewServer(f.s.Surface())
	t.Cleanup(srv.Close)
	events := openStream(g, t, srv.URL+"/events")
	g.Eventually(events).Should(Receive(Equal("retry: 2000")))
	f.comment(g, t, id, "A point.")
	f.decide(g, t, id, store.VerdictApprove, "")
	g.Consistently(events, 300*time.Millisecond).ShouldNot(Receive(HavePrefix("data:")))
}

// openStream reads a stream's lines into a channel, skipping blank ones.
func openStream(g *WithT, t *testing.T, url string) <-chan string {
	g.THelper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	g.Expect(err).NotTo(HaveOccurred())
	res, err := http.DefaultClient.Do(req)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res.Header.Get("Content-Type")).To(HavePrefix("text/event-stream"))
	out := make(chan string, 16)
	go func() {
		defer func() { _ = res.Body.Close() }()
		scan := bufio.NewScanner(res.Body)
		for scan.Scan() {
			if line := strings.TrimSpace(scan.Text()); line != "" {
				out <- line
			}
		}
	}()
	return out
}
