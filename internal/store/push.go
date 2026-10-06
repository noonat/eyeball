package store

import (
	"context"
	"database/sql"

	"github.com/cockroachdb/errors"
)

// Subscription is one device's web push subscription.
type Subscription struct {
	// Endpoint is the push service's address for the device.
	Endpoint string
	// P256dh and Auth are the device's keys, which encrypt what is sent.
	P256dh, Auth string
}

// What a device is told about its subscription.
const (
	// SubscriptionActive is a subscription pushes still go to.
	SubscriptionActive = "active"
	// SubscriptionLapsed is one a push service called gone.
	SubscriptionLapsed = "lapsed"
)

// VAPID reads the daemon's key pair, generating and keeping one with generate
// the first time. A new pair would orphan every subscription, so it is made
// once.
func (s *Store) VAPID(ctx context.Context, generate func() (public, private string, err error)) (string, string, error) {
	var public, private string
	err := s.tx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT public, private FROM vapid WHERE id = 1")
		err := row.Scan(&public, &private)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return errors.Wrap(err, "read vapid keys")
		}
		if public, private, err = generate(); err != nil {
			return errors.Wrap(err, "generate vapid keys")
		}
		insert := "INSERT INTO vapid (id, public, private, created) VALUES (1, ?, ?, ?)"
		_, err = tx.ExecContext(ctx, insert, public, private, FormatTime(s.now()))
		return errors.Wrap(err, "keep vapid keys")
	})
	return public, private, err
}

// Subscribe stores a device's subscription, or confirms one already held, and
// says whether it is active or lapsed. A known endpoint keeps its lapse: a
// device that posts a dead subscription must be told it is dead.
func (s *Store) Subscribe(ctx context.Context, sub Subscription) (string, error) {
	state := SubscriptionActive
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var lapsed string
		row := tx.QueryRowContext(ctx, "SELECT lapsed FROM subscriptions WHERE endpoint = ?", sub.Endpoint)
		err := row.Scan(&lapsed)
		at := FormatTime(s.now())
		switch {
		case errors.Is(err, sql.ErrNoRows):
			insert := `INSERT INTO subscriptions (endpoint, p256dh, auth, created, confirmed)
				VALUES (?, ?, ?, ?, ?)`
			_, err = tx.ExecContext(ctx, insert, sub.Endpoint, sub.P256dh, sub.Auth, at, at)
			return errors.Wrap(err, "insert subscription")
		case err != nil:
			return errors.Wrap(err, "read subscription")
		}
		if lapsed != "" {
			state = SubscriptionLapsed
		}
		update := "UPDATE subscriptions SET p256dh = ?, auth = ?, confirmed = ? WHERE endpoint = ?"
		_, err = tx.ExecContext(ctx, update, sub.P256dh, sub.Auth, at, sub.Endpoint)
		return errors.Wrap(err, "confirm subscription")
	})
	return state, err
}

// ActiveSubscriptions lists every subscription not marked lapsed.
func (s *Store) ActiveSubscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT endpoint, p256dh, auth FROM subscriptions WHERE lapsed = '' ORDER BY id")
	if err != nil {
		return nil, errors.Wrap(err, "read subscriptions")
	}
	defer func() { _ = rows.Close() }()
	var out []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.Endpoint, &sub.P256dh, &sub.Auth); err != nil {
			return nil, errors.Wrap(err, "scan subscription")
		}
		out = append(out, sub)
	}
	return out, errors.Wrap(rows.Err(), "read subscriptions")
}

// MarkLapsed records that a push service called a subscription gone. It is
// not retried, and its device is told so the next time it posts.
func (s *Store) MarkLapsed(ctx context.Context, endpoint string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE subscriptions SET lapsed = ? WHERE endpoint = ? AND lapsed = ''", FormatTime(s.now()), endpoint)
	return errors.Wrap(err, "mark subscription lapsed")
}
