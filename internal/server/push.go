package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

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
