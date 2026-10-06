package push

import (
	"context"
	"encoding/json"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/cockroachdb/errors"
)

// Subscription is the device a push goes to.
type Subscription struct {
	// Endpoint is the push service's address for the device.
	Endpoint string
	// P256dh and Auth are the device's keys, which encrypt the payload.
	P256dh, Auth string
}

// Payload is what the service worker shows: the notification's title and
// body, the round it opens, and the count the icon's badge shows.
type Payload struct {
	// Title is the review's title.
	Title string `json:"title"`
	// Body names the project and the round.
	Body string `json:"body"`
	// URL is the round's address, relative to the surface.
	URL string `json:"url"`
	// Badge is how many reviews wait, for the icon's badge.
	Badge int `json:"badge"`
}

// Keys are the daemon's VAPID pair and the subscriber the push services are
// told is sending, the surface's https address.
type Keys struct {
	// Public and Private are the VAPID pair.
	Public, Private string
	// Subscriber is the surface's https address.
	Subscriber string
}

// ttl is how long a push service holds a notification for a device that is
// off: a day, since a review asked for overnight is still waiting in the
// morning.
const ttl = 24 * 60 * 60

// Send pushes one payload to one device and returns the push service's
// status. A 404 or 410 means the subscription is gone.
func Send(ctx context.Context, sub Subscription, p Payload, k Keys) (int, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return 0, errors.Wrap(err, "encode the payload")
	}
	res, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		Subscriber:      k.Subscriber,
		VAPIDPublicKey:  k.Public,
		VAPIDPrivateKey: k.Private,
		TTL:             ttl,
		Urgency:         webpush.UrgencyHigh,
	})
	if err != nil {
		return 0, errors.Wrap(err, "send the push")
	}
	_ = res.Body.Close()
	return res.StatusCode, nil
}

// Gone reports a status that means the subscription no longer exists.
func Gone(status int) bool {
	return status == 404 || status == 410
}
