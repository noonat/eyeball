package render

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestAge(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{ago: -time.Hour, want: "now"},
		{ago: 0, want: "now"},
		{ago: 59 * time.Second, want: "now"},
		{ago: time.Minute, want: "1m"},
		{ago: 59*time.Minute + 59*time.Second, want: "59m"},
		{ago: time.Hour, want: "1h"},
		{ago: 23*time.Hour + 59*time.Minute, want: "23h"},
		{ago: 24 * time.Hour, want: "Yesterday"},
		{ago: 47*time.Hour + 59*time.Minute, want: "Yesterday"},
		{ago: 48 * time.Hour, want: "2d"},
		{ago: 6*24*time.Hour + 23*time.Hour, want: "6d"},
		{ago: 7 * 24 * time.Hour, want: "Sep 28"},
		{ago: 400 * 24 * time.Hour, want: "Aug 31, 2025"},
	}
	for _, c := range cases {
		t.Run(c.want+" at "+c.ago.String(), func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(Age(now, now.Add(-c.ago))).To(Equal(c.want))
		})
	}
}

func TestAge_dateInTheViewersZone(t *testing.T) {
	g := NewWithT(t)
	// Late on Sep 27 in Los Angeles is already Sep 28 in UTC. The date is the
	// one the clock showing now would show.
	la, err := time.LoadLocation("America/Los_Angeles")
	g.Expect(err).NotTo(HaveOccurred())
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, la)
	then := time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC)
	g.Expect(Age(now, then)).To(Equal("Sep 27"))
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{ago: 59 * time.Second, want: "just now"},
		{ago: time.Minute, want: "1 minute ago"},
		{ago: 59 * time.Minute, want: "59 minutes ago"},
		{ago: time.Hour, want: "1 hour ago"},
		{ago: 23 * time.Hour, want: "23 hours ago"},
		{ago: 24 * time.Hour, want: "yesterday"},
		{ago: 48 * time.Hour, want: "2 days ago"},
		{ago: 6 * 24 * time.Hour, want: "6 days ago"},
		{ago: 7 * 24 * time.Hour, want: "on Sep 28"},
		{ago: 400 * 24 * time.Hour, want: "on Aug 31, 2025"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(Ago(now, now.Add(-c.ago))).To(Equal(c.want))
		})
	}
}
