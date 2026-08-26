package store

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	. "github.com/onsi/gomega"
)

func Test_migrate(t *testing.T) {
	g := NewWithT(t)
	db := openDB(g, t)
	files := fstest.MapFS{
		"0001_widgets.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL)"),
		},
	}

	g.Expect(migrate(t.Context(), db, files, fixedClock())).To(Succeed())

	// The table is there, and the ledger records the file with the sha256 of
	// its bytes and when it ran.
	_, err := db.ExecContext(t.Context(), "INSERT INTO widgets (name) VALUES ('one')")
	g.Expect(err).NotTo(HaveOccurred())

	applied, err := appliedMigrations(t.Context(), db)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(applied).To(HaveLen(1))
	carried, err := readMigrations(files)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(applied["0001_widgets.sql"]).To(Equal(carried[0].Sum))

	var at string
	row := db.QueryRowContext(t.Context(), "SELECT applied FROM schema_migrations")
	g.Expect(row.Scan(&at)).To(Succeed())
	g.Expect(at).To(Equal(FormatTime(fixedClock()())))

	// Running again applies nothing and reports nothing.
	g.Expect(migrate(t.Context(), db, files, fixedClock())).To(Succeed())
	applied, err = appliedMigrations(t.Context(), db)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(applied).To(HaveLen(1))
}

func Test_migrateAppliesInOrder(t *testing.T) {
	g := NewWithT(t)
	db := openDB(g, t)
	// 0002 depends on the table 0001 creates, so applying them out of order
	// fails rather than passing quietly. 0001 also holds two statements, which
	// is what a real schema migration looks like.
	files := fstest.MapFS{
		"0002_second.sql": &fstest.MapFile{
			Data: []byte("INSERT INTO widgets (name) VALUES ('from 0002')"),
		},
		"0001_first.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL);\nCREATE INDEX widgets_name ON widgets (name);"),
		},
	}

	g.Expect(migrate(t.Context(), db, files, fixedClock())).To(Succeed())

	var name string
	row := db.QueryRowContext(t.Context(), "SELECT name FROM widgets")
	g.Expect(row.Scan(&name)).To(Succeed())
	g.Expect(name).To(Equal("from 0002"))

	// The index proves the second statement in 0001 ran, not only the first.
	var count int
	row = db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'widgets_name'")
	g.Expect(row.Scan(&count)).To(Succeed())
	g.Expect(count).To(Equal(1))
}

func Test_migrateFromTwoProcessesAtOnce(t *testing.T) {
	g := NewWithT(t)
	// Two handles are two writers as far as SQLite is concerned. Both read an
	// empty ledger, so without a recheck inside the transaction the loser runs
	// CREATE TABLE against a schema the winner has already built.
	//
	// The ledger being created outside a transaction failed here too, as
	// SQLITE_BUSY on a read lock trying to upgrade, which busy_timeout does not
	// retry.
	path := filepath.Join(t.TempDir(), "eyeball.db")
	// Put the file into WAL first. Setting the journal mode on a database that
	// does not exist yet takes an exclusive lock, and two openers racing that
	// is a different race from the one under test. Conflating the two made this
	// fail about twice in ten runs.
	warm, err := sql.Open("sqlite", dsn(path))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warm.PingContext(t.Context())).To(Succeed())
	g.Expect(warm.Close()).To(Succeed())

	files := fstest.MapFS{
		"0001_widgets.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY)"),
		},
	}

	const openers = 4
	var wg sync.WaitGroup
	errs := make([]error, openers)
	for i := range openers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := sql.Open("sqlite", dsn(path))
			if err != nil {
				errs[i] = err
				return
			}
			defer func() { _ = db.Close() }()
			db.SetMaxOpenConns(1)
			errs[i] = migrate(t.Context(), db, files, fixedClock())
		}()
	}
	wg.Wait()
	for i := range openers {
		g.Expect(errs[i]).NotTo(HaveOccurred())
	}

	// Applied once, recorded once.
	check, err := sql.Open("sqlite", dsn(path))
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = check.Close() }()
	var count int
	row := check.QueryRowContext(t.Context(), "SELECT count(*) FROM schema_migrations")
	g.Expect(row.Scan(&count)).To(Succeed())
	g.Expect(count).To(Equal(1))
}
func Test_migrateRefusesAChangedFile(t *testing.T) {
	g := NewWithT(t)
	db := openDB(g, t)
	before := fstest.MapFS{
		"0001_widgets.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY)"),
		},
	}
	g.Expect(migrate(t.Context(), db, before, fixedClock())).To(Succeed())

	after := fstest.MapFS{
		"0001_widgets.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)"),
		},
	}
	err := migrate(t.Context(), db, after, fixedClock())
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("0001_widgets.sql"))
	g.Expect(err.Error()).To(ContainSubstring("changed after it ran"))
}

func Test_migrateRefusesADowngrade(t *testing.T) {
	g := NewWithT(t)
	db := openDB(g, t)
	newer := fstest.MapFS{
		"0001_widgets.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY)"),
		},
		"0002_gadgets.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE gadgets (id INTEGER PRIMARY KEY)"),
		},
	}
	g.Expect(migrate(t.Context(), db, newer, fixedClock())).To(Succeed())

	// An older binary carries only the first file, so it is about to run
	// against a schema it does not know.
	older := fstest.MapFS{
		"0001_widgets.sql": newer["0001_widgets.sql"],
	}
	err := migrate(t.Context(), db, older, fixedClock())
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("0002_gadgets.sql"))
	g.Expect(err.Error()).To(ContainSubstring("newer eyeball"))
}

func Test_migrateSkipsWhatIsNotSQL(t *testing.T) {
	g := NewWithT(t)
	// The embedded directory carries a README, which must not be handed to the
	// database as a statement.
	carried, err := readMigrations(migrationFS())
	g.Expect(err).NotTo(HaveOccurred())
	for _, m := range carried {
		g.Expect(m.Name).To(HaveSuffix(".sql"))
	}

	db := openDB(g, t)
	g.Expect(migrate(t.Context(), db, migrationFS(), fixedClock())).To(Succeed())
}

// fixedClock returns a clock that does not move, so a recorded timestamp is
// something a test can assert rather than approximate.
func fixedClock() func() time.Time {
	at := time.Date(2026, 8, 26, 4, 5, 6, 700_000_000, time.UTC)
	return func() time.Time { return at }
}

// openDB opens a database with no migrations applied, for a test that drives
// the runner directly. It is a file rather than :memory: because the pragmas
// are most of what Open delivers and an in-memory database refuses WAL.
func openDB(g *WithT, t *testing.T) *sql.DB {
	g.THelper()
	db, err := sql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "eyeball.db")))
	g.Expect(err).NotTo(HaveOccurred())
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	g.Expect(db.PingContext(t.Context())).To(Succeed())
	return db
}
