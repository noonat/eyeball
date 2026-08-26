package store

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// TimeLayout is how every column holding a timestamp is written: RFC3339 with
// three fractional digits, in UTC, ending in a literal Z.
//
// The width is fixed so that lexical order and chronological order agree, which
// is what lets a query sort on the text. time.RFC3339Nano trims trailing zeros
// and breaks that on exactly the values a test is most likely to produce.
const TimeLayout = "2006-01-02T15:04:05.000Z"

// pragmas are set on every connection the pool opens, in this order.
//
// busy_timeout is first because what follows it can block. Setting the journal
// mode on a database another connection is opening takes an exclusive lock, and
// with no timeout yet in force that returns SQLITE_BUSY at once instead of
// waiting.
//
// synchronous=FULL rather than the usual NORMAL: losing a comment is the
// failure this must not have, and under WAL the default is durable against a
// process crash but not against the machine losing power. The cost is an fsync
// per write transaction, on a workload of a few writes a minute.
var pragmas = []string{
	"busy_timeout(5000)",
	"journal_mode(WAL)",
	"synchronous(FULL)",
	"foreign_keys(1)",
}

// Store is the SQLite database holding reviews, rounds, comments and decisions.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens the database at path and applies every migration it does not
// already have. The path must be absolute, so that what it names does not
// depend on the working directory of whoever opened it.
func Open(ctx context.Context, path string) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.Newf("database path %q is relative; pass an absolute path", path)
	}
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, errors.Wrap(err, "open database")
	}
	// One connection, so writers never contend with each other in this process
	// and SQLITE_BUSY needs no retry here. Under WAL a reader does not block
	// the writer, so a reader pool is what this buys back if a read ever grows
	// slow enough to stall a write.
	db.SetMaxOpenConns(1)

	now := func() time.Time { return time.Now().UTC() }
	if err := migrate(ctx, db, migrationFS(), now); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, now: now}, nil
}

// Close releases the connection pool.
func (s *Store) Close() error {
	return errors.Wrap(s.db.Close(), "close database")
}

// FormatTime writes t in TimeLayout. It converts to UTC first, because the
// layout ends in a literal Z and would otherwise stamp a local wall clock as
// though it were UTC.
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// ParseTime reads a timestamp written by FormatTime.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(TimeLayout, s)
	return t, errors.Wrapf(err, "parse timestamp %q", s)
}

// dsn is the connection string for the database at path.
//
// The pragmas go here rather than into an Exec after opening, because they are
// per-connection and database/sql opens a connection whenever it wants one. A
// pragma set once applies to that connection and silently not to the next.
//
// The URL is built rather than pasted together. Measured against this driver, a
// path holding a ? breaks a concatenated string: the database opens with
// journal_mode=delete instead of WAL, and nothing reports it. Building the URL
// escapes the path, so the query starts where it is meant to.
func dsn(path string) string {
	query := url.Values{}
	for _, pragma := range pragmas {
		query.Add("_pragma", pragma)
	}
	// _txlock applies to every transaction this handle begins, read-only ones
	// included, because there is no way to ask database/sql for a deferred one.
	// It is what keeps a read-then-write check correct if the pool ever widens
	// past one connection, or a second process opens the file.
	query.Set("_txlock", "immediate")

	dsn := url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}
	return dsn.String()
}
