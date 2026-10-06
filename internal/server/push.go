package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/noonat/eyeball/internal/push"
	"github.com/noonat/eyeball/internal/store"
)

// vapidPublic is the daemon's public VAPID key, generated on first use. A key
// that cannot be read leaves the page unable to subscribe, which its first-run
// steps then show, rather than failing the page.
func (s *Server) vapidPublic() string {
	s.vapidMu.Lock()
	defer s.vapidMu.Unlock()
	if s.vapidKey != "" {
		return s.vapidKey
	}
	public, _, err := s.db.s.VAPID(context.Background(), push.GenerateKeys)
	if err != nil {
		s.log.Error("read the vapid keys", "err", err)
		return ""
	}
	s.vapidKey = public
	return public
}

// subscriptionBody is PushSubscription.toJSON() as a browser sends it.
type subscriptionBody struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// handleSubscribe answers POST /subscriptions: a device's push subscription,
// posted on every page load, stored or confirmed, and answered with its state.
// Posting on every load is how a device learns its subscription lapsed, since
// no push reaches a dead one to say so.
func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) error {
	var body subscriptionBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if err := dec.Decode(&body); err != nil || !validSubscription(body) {
		plain(w, http.StatusBadRequest, "That is not a push subscription.")
		return nil
	}
	state, err := s.db.s.Subscribe(r.Context(), store.Subscription{Endpoint: body.Endpoint, P256dh: body.Keys.P256dh, Auth: body.Keys.Auth})
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(map[string]string{"state": state})
}

// validSubscription checks what a push needs: an absolute endpoint and both
// keys. Push services' endpoints are https; plain http is allowed for a local
// one, which is what the tests run.
func validSubscription(b subscriptionBody) bool {
	u, err := url.Parse(b.Endpoint)
	if err != nil || u.Host == "" {
		return false
	}
	local := u.Scheme == "http" && strings.HasPrefix(u.Host, "127.0.0.1:")
	return (u.Scheme == "https" || local) && b.Keys.P256dh != "" && b.Keys.Auth != ""
}

// pushTimeout bounds one push, so a push service that hangs holds nothing for
// long.
const pushTimeout = 10 * time.Second

// notify sends one notification for a request to every active subscription,
// from a goroutine, so a push service that is slow or down never holds the
// agent's answer. Comments and verdicts send none: they travel toward an agent
// blocked on a read, not toward the reviewer.
func (s *Server) notify(review store.Review, round store.Round) {
	s.pushes.Add(1)
	go func() {
		defer s.pushes.Done()
		if err := s.sendPushes(review, round); err != nil {
			s.log.Error("send notifications", "review", review.ID, "err", err)
		}
	}()
}

// sendPushes does notify's work. A surface with no tailnet address sends
// nothing: the subscriber push services are told about is that address, and
// no installed page subscribed without one.
func (s *Server) sendPushes(review store.Review, round store.Round) error {
	if s.surface == nil {
		return nil
	}
	address := s.surface().URL
	if address == "" {
		return nil
	}
	ctx := context.Background()
	subs, err := s.db.s.ActiveSubscriptions(ctx)
	if err != nil || len(subs) == 0 {
		return err
	}
	public, private, err := s.db.s.VAPID(ctx, push.GenerateKeys)
	if err != nil {
		return err
	}
	project, err := s.db.s.Project(ctx, review.ProjectID)
	if err != nil {
		return err
	}
	q, err := s.db.s.Queue(ctx, 0)
	if err != nil {
		return err
	}
	p := push.Payload{
		Title: review.Title,
		Body:  fmt.Sprintf("%s · Round %d", project.Name, round.Number),
		URL:   fmt.Sprintf("/reviews/%d", review.ID),
		Badge: len(q.Waiting),
	}
	keys := push.Keys{Public: public, Private: private, Subscriber: address}
	for _, sub := range subs {
		sendCtx, cancel := context.WithTimeout(ctx, pushTimeout)
		status, err := push.Send(sendCtx, push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, p, keys)
		cancel()
		switch {
		case err != nil:
			s.log.Warn("push failed", "endpoint", sub.Endpoint, "err", err)
		case push.Gone(status):
			if err := s.db.s.MarkLapsed(ctx, sub.Endpoint); err != nil {
				return err
			}
		case status >= 300:
			// A push service briefly down says nothing about the device, so
			// the subscription stays active.
			s.log.Warn("push refused", "endpoint", sub.Endpoint, "status", status)
		}
	}
	return nil
}

// WaitPushes waits for notifications being sent, until ctx ends. Shutdown
// calls it within its grace, so a push in flight is not cut off at stop.
func (s *Server) WaitPushes(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.pushes.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
