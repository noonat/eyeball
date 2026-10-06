package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/store"
)

// keepalive is how often an idle stream sends a comment line, so a page can
// tell an idle stream from a dead one. Fifteen seconds is a guess.
const keepalive = 15 * time.Second

// handleEvents answers GET /events: a Server-Sent Events stream whose state is
// the page's connection.
//
// The hub publishes on every write in every project, and a page that reloaded
// on each signal would reload constantly, mid-sentence. So a stream sends a
// stamp of what its page shows, recomputed after each signal and sent only
// when it moved. /events?review=N stamps a review, /events?queue=1 the queue,
// and plain /events, which every other page opens, sends keepalives alone.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("the response cannot be flushed, so it cannot stream")
	}
	stamp, err := s.streamStamp(r)
	if err != nil {
		return err
	}
	// Subscribed before the first stamp is read, so a change between the read
	// and the wait is not missed.
	signals, stop := s.hub.subscribe()
	defer stop()

	ctx := r.Context()
	last := ""
	if stamp != nil {
		if last, err = stamp(ctx); err != nil {
			return err
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 2000\n\n")
	// The running build, on every connect. A stream reconnects by itself after
	// a daemon restart, so a page from the old daemon hears the new build and
	// offers to reload.
	_, _ = fmt.Fprintf(w, "event: build\ndata: %s\n\n", s.daemon.Build)
	if last != "" {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", last)
	}
	flusher.Flush()

	tick := time.NewTicker(keepalive)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		// A stream that ignored the daemon stopping would hold
		// http.Server.Shutdown for its whole grace, as a wait would.
		case <-s.stopping:
			return nil
		case <-tick.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case <-signals:
			if stamp == nil {
				continue
			}
			now, err := stamp(ctx)
			if err != nil {
				return err
			}
			if now != last {
				last = now
				_, _ = fmt.Fprintf(w, "data: %s\n\n", now)
				flusher.Flush()
			}
		}
	}
}

// streamStamp chooses the stamp a stream sends from its query, and nil for a
// stream that only keeps the connection.
func (s *Server) streamStamp(r *http.Request) (func(context.Context) (string, error), error) {
	q := r.URL.Query()
	if v := q.Get("review"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, errors.Newf("review %q is not a review's id", v)
		}
		return func(ctx context.Context) (string, error) { return s.reviewStamp(ctx, id) }, nil
	}
	if q.Get("queue") == "1" {
		return s.queueStamp, nil
	}
	return nil, nil
}

// reviewStamp is a hash of what a review page shows that someone other than
// the page's own reader can change: the review's state, its latest round and
// whether that round is decided, and every reply.
//
// A comment count is left out, since the reader's own comment would reload
// the page that wrote it. Reply texts stand in for the time each was written:
// store.Reply replaces an earlier reply, so a count of replies would not move
// when one changes.
func (s *Server) reviewStamp(ctx context.Context, id int64) (string, error) {
	review, err := s.db.s.Review(ctx, id)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%d", review.State, review.LatestRound)
	for n := 1; n <= review.LatestRound; n++ {
		round, err := s.db.s.Round(ctx, id, n)
		if err != nil {
			return "", err
		}
		_, err = s.db.Decision(ctx, round.ID)
		decided := err == nil
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
		_, _ = fmt.Fprintf(h, "|%d:%t", n, decided)
		comments, err := s.db.s.Comments(ctx, round.ID)
		if err != nil {
			return "", err
		}
		for _, c := range comments {
			if c.Reply != "" {
				_, _ = fmt.Fprintf(h, "|%d=%s", c.ID, c.Reply)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:8]), nil
}

// queueStamp is a hash of every row the queue lists, by what decides where it
// sits and what it says.
func (s *Server) queueStamp(ctx context.Context) (string, error) {
	q, err := s.db.s.Queue(ctx, settledShown)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	groups := [][]store.ReviewRow{q.Waiting, q.Working, q.Settled}
	for _, group := range groups {
		_, _ = fmt.Fprint(h, "#")
		for _, row := range group {
			_, _ = fmt.Fprintf(h, "|%d:%s:%d:%s", row.ReviewID, row.State, row.Round, row.Verdict)
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:8]), nil
}
