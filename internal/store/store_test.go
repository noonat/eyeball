package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestFormatTime(t *testing.T) {
	moments := []struct {
		name string
		at   time.Time
		want string
	}{
		{
			name: "a whole second keeps its three fractional digits",
			at:   time.Date(2026, 8, 26, 4, 5, 6, 0, time.UTC),
			want: "2026-08-26T04:05:06.000Z",
		},
		{
			name: "a trailing zero is kept, which is what fixes the width",
			at:   time.Date(2026, 8, 26, 4, 5, 6, 700_000_000, time.UTC),
			want: "2026-08-26T04:05:06.700Z",
		},
		{
			name: "a local time is converted rather than relabeled",
			at:   time.Date(2026, 8, 26, 6, 5, 6, 0, time.FixedZone("east", 2*3600)),
			want: "2026-08-26T04:05:06.000Z",
		},
	}
	for _, c := range moments {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got := FormatTime(c.at)
			g.Expect(got).To(Equal(c.want))
			g.Expect(got).To(HaveLen(len(TimeLayout)))
		})
	}
}

func TestFormatTime_ordersLexically(t *testing.T) {
	g := NewWithT(t)
	// The point of the fixed width: a text sort is a chronological sort, which
	// is what lets the queue order on the column.
	earlier := time.Date(2026, 8, 26, 4, 5, 6, 0, time.UTC)
	later := earlier.Add(time.Millisecond)
	g.Expect(FormatTime(earlier) < FormatTime(later)).To(BeTrue())

	// time.RFC3339Nano is the trap this layout exists to avoid: it drops the
	// trailing zeros, so the shorter string sorts after the longer one.
	g.Expect(earlier.Format(time.RFC3339Nano) < later.Format(time.RFC3339Nano)).To(BeFalse())
}

func TestOpen(t *testing.T) {
	g := NewWithT(t)
	path := filepath.Join(t.TempDir(), "eyeball.db")

	store, err := Open(t.Context(), path)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { g.Expect(store.Close()).To(Succeed()) }()

	g.Expect(path).To(BeARegularFile())
	g.Expect(store.now).NotTo(BeNil())
	g.Expect(store.now()).To(BeTemporally("~", time.Now().UTC(), time.Minute))

	// Opening the same file again applies nothing and still succeeds, which is
	// what every CLI command does after the first.
	again, err := Open(t.Context(), path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(again.Close()).To(Succeed())
}

func TestOpen_pragmas(t *testing.T) {
	// A misspelled pragma in the DSN is accepted and ignored, and the first
	// query still succeeds, so a setting that did not apply looks exactly like
	// one that did. Each is read back rather than assumed. Only synchronous
	// cannot be proved this way, because 2 is also the default.
	wanted := []struct {
		pragma string
		want   string
	}{
		{
			pragma: "journal_mode",
			want:   "wal",
		},
		{
			pragma: "synchronous",
			want:   "2",
		},
		{
			pragma: "foreign_keys",
			want:   "1",
		},
		{
			pragma: "busy_timeout",
			want:   "5000",
		},
	}
	for _, c := range wanted {
		t.Run(c.pragma, func(t *testing.T) {
			g := NewWithT(t)
			store, err := Open(t.Context(), filepath.Join(t.TempDir(), "eyeball.db"))
			g.Expect(err).NotTo(HaveOccurred())
			defer func() { g.Expect(store.Close()).To(Succeed()) }()

			var got string
			row := store.db.QueryRowContext(t.Context(), "PRAGMA "+c.pragma)
			g.Expect(row.Scan(&got)).To(Succeed())
			g.Expect(got).To(Equal(c.want))
		})
	}
}

func TestOpen_pragmasAreThoseIntended(t *testing.T) {
	g := NewWithT(t)
	// A misspelled pragma is accepted and ignored, and synchronous=FULL is
	// SQLite's own default, so no read-back can tell a typo in that one from a
	// setting that applied. The list is pinned here instead, which makes
	// changing it deliberate. See TestOpen_pragmas for the three whose defaults
	// differ, where the read-back does prove the DSN was honored.
	intended := []string{
		"busy_timeout(5000)",
		"journal_mode(WAL)",
		"synchronous(FULL)",
		"foreign_keys(1)",
	}
	g.Expect(pragmas).To(Equal(intended))
}

func TestOpen_pragmasSurviveAnAwkwardPath(t *testing.T) {
	g := NewWithT(t)
	// A path holding a ? breaks a concatenated DSN. Building the URL escapes
	// the path instead, so a directory named like this is the test for it.
	dir := filepath.Join(t.TempDir(), "we?ird#dir")
	g.Expect(os.MkdirAll(dir, 0o700)).To(Succeed())

	store, err := Open(t.Context(), filepath.Join(dir, "eyeball.db"))
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { g.Expect(store.Close()).To(Succeed()) }()

	// journal_mode is the one that goes: a concatenated DSN over this path
	// opens the database with delete rather than WAL. busy_timeout survives it,
	// so asserting that instead would pass either way.
	var got string
	row := store.db.QueryRowContext(t.Context(), "PRAGMA journal_mode")
	g.Expect(row.Scan(&got)).To(Succeed())
	g.Expect(got).To(Equal("wal"))
}

func TestOpen_relativePath(t *testing.T) {
	g := NewWithT(t)
	// What a relative path names depends on the working directory of whoever
	// opened it, which is not something a daemon should inherit silently.
	_, err := Open(t.Context(), filepath.Join("relative", "eyeball.db"))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("relative"))
}

func TestParseTime(t *testing.T) {
	g := NewWithT(t)
	at := time.Date(2026, 8, 26, 4, 5, 6, 700_000_000, time.UTC)

	got, err := ParseTime(FormatTime(at))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeTemporally("==", at))

	_, err = ParseTime("26 August 2026")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("26 August 2026"))
}
