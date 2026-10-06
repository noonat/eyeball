package store

import (
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"
)

// A database from before 0007 reads forward with its reviews intact.
func Test_migrateAddsPushToAnOlderDatabase(t *testing.T) {
	g := NewWithT(t)
	db := openDB(g, t)
	all, err := readMigrations(migrationFS())
	g.Expect(err).NotTo(HaveOccurred())
	before := fstest.MapFS{}
	for _, m := range all {
		if m.Name < "0007" {
			before[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
		}
	}
	g.Expect(migrate(t.Context(), db, before, fixedClock())).To(Succeed())
	_, err = db.ExecContext(t.Context(), "INSERT INTO projects (root, name, created) VALUES ('/tmp/p', 'p', '2026-08-26T04:05:06.000Z')")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(migrate(t.Context(), db, migrationFS(), fixedClock())).To(Succeed())
	var projects, subs int
	g.Expect(db.QueryRowContext(t.Context(), "SELECT (SELECT count(*) FROM projects), (SELECT count(*) FROM subscriptions)").Scan(&projects, &subs)).To(Succeed())
	g.Expect([]int{projects, subs}).To(Equal([]int{1, 0}))
}

// A device learns its subscription lapsed the next time it posts it.
func TestStore_Subscribe(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	a := Subscription{Endpoint: "https://push.example/a", P256dh: "k1", Auth: "a1"}
	b := Subscription{Endpoint: "https://push.example/b", P256dh: "k2", Auth: "a2"}
	posts := []Subscription{a, b, a}
	for _, sub := range posts {
		state, err := s.Subscribe(t.Context(), sub)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(state).To(Equal(SubscriptionActive))
	}
	active, err := s.ActiveSubscriptions(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(active).To(Equal([]Subscription{a, b}))

	g.Expect(s.MarkLapsed(t.Context(), a.Endpoint)).To(Succeed())
	state, err := s.Subscribe(t.Context(), a)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(state).To(Equal(SubscriptionLapsed))
	active, err = s.ActiveSubscriptions(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(active).To(Equal([]Subscription{b}))
}

// The key pair is made once: a second pair would orphan every subscription.
func TestStore_VAPID(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	calls := 0
	generate := func() (string, string, error) {
		calls++
		return "public-key", "private-key", nil
	}
	pub, priv, err := s.VAPID(t.Context(), generate)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect([]string{pub, priv}).To(Equal([]string{"public-key", "private-key"}))
	pub, _, err = s.VAPID(t.Context(), generate)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pub).To(Equal("public-key"))
	g.Expect(calls).To(Equal(1))
}
