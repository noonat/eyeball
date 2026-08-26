package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

//go:embed migrations
var migrations embed.FS

// migrationDir is where the embedded files sit inside migrations.
const migrationDir = "migrations"

// ledgerDDL creates the table recording which migrations have run. It is the
// runner's own bookkeeping rather than a migration, because the runner has to
// read it before it can apply anything.
const ledgerDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
	name    TEXT NOT NULL PRIMARY KEY,
	sha256  TEXT NOT NULL,
	applied TEXT NOT NULL
)`

// insertLedger records one applied migration.
const insertLedger = `INSERT INTO schema_migrations (name, sha256, applied) VALUES (?, ?, ?)`

// migration is one *.sql file to apply.
type migration struct {
	// Name is the file's name, which is also its identity in the ledger.
	Name string
	// SQL is the file's contents.
	SQL string
	// Sum is the sha256 of the file's bytes, written as hex.
	Sum string
}

// migrationFS is the embedded migration directory, rooted at its contents.
func migrationFS() fs.FS {
	sub, err := fs.Sub(migrations, migrationDir)
	if err != nil {
		// The directory is embedded by a compile-time directive, so a failure
		// here would mean the binary was built without it.
		panic(err)
	}
	return sub
}

// migrate applies every migration in fsys that this database has not run.
//
// Forward only, in lexical order, each in its own transaction. Two things fail
// the open rather than being repaired. A recorded file whose bytes have changed
// means two databases can share a ledger and hold different schemas, with
// nothing to tell them apart. A recorded migration this binary does not carry
// means old code is about to run against a newer schema.
func migrate(ctx context.Context, db *sql.DB, fsys fs.FS, now func() time.Time) error {
	if err := createLedger(ctx, db); err != nil {
		return err
	}
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}
	carried, err := readMigrations(fsys)
	if err != nil {
		return err
	}
	if err := refuseDowngrade(applied, carried); err != nil {
		return err
	}
	for _, m := range carried {
		sum, ran := applied[m.Name]
		if !ran {
			if err := apply(ctx, db, m, now); err != nil {
				return err
			}
			continue
		}
		if sum != m.Sum {
			return errors.Newf("migration %s changed after it ran; add a new migration, or delete the database while nothing has shipped", m.Name)
		}
	}
	return nil
}

// createLedger makes the bookkeeping table, inside a transaction.
//
// A bare Exec takes a read lock to look at the schema and then upgrades it to a
// write, and busy_timeout does not retry an upgrade: two processes opening a
// fresh database at once get SQLITE_BUSY rather than waiting for each other.
// BEGIN IMMEDIATE takes the write lock up front, which the timeout does cover.
func createLedger(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "begin migration ledger")
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, ledgerDDL); err != nil {
		return errors.Wrap(err, "create migration ledger")
	}
	return errors.Wrap(tx.Commit(), "commit migration ledger")
}

// refuseDowngrade reports a migration the database has run that this binary does
// not carry, which is old code about to run against a newer schema.
func refuseDowngrade(applied map[string]string, carried []migration) error {
	have := make(map[string]struct{}, len(carried))
	for _, m := range carried {
		have[m.Name] = struct{}{}
	}
	var missing []string
	for name := range applied {
		if _, ok := have[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	return errors.Newf("this database has run %s, which this binary does not carry; it was written by a newer eyeball", strings.Join(missing, ", "))
}

// apply runs one migration and records it in a single transaction, so a
// half-applied migration cannot be recorded as done.
func apply(ctx context.Context, db *sql.DB, m migration, now func() time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "begin migration %s", m.Name)
	}
	defer func() { _ = tx.Rollback() }()

	// The ledger was read before this transaction opened, so another process
	// may have applied this file in between. BEGIN IMMEDIATE serializes the
	// two, which is what makes the recheck reliable: without it the second
	// process fails on a table the first has just created.
	var already int
	row := tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE name = ?", m.Name)
	if err := row.Scan(&already); err != nil {
		return errors.Wrapf(err, "recheck migration %s", m.Name)
	}
	if already > 0 {
		return nil
	}

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return errors.Wrapf(err, "apply migration %s", m.Name)
	}
	if _, err := tx.ExecContext(ctx, insertLedger, m.Name, m.Sum, FormatTime(now())); err != nil {
		return errors.Wrapf(err, "record migration %s", m.Name)
	}
	return errors.Wrapf(tx.Commit(), "commit migration %s", m.Name)
}

// appliedMigrations maps each migration this database has run to the sha256
// recorded for it.
func appliedMigrations(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT name, sha256 FROM schema_migrations")
	if err != nil {
		return nil, errors.Wrap(err, "read migration ledger")
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var name, sum string
		if err := rows.Scan(&name, &sum); err != nil {
			return nil, errors.Wrap(err, "scan migration ledger")
		}
		out[name] = sum
	}
	return out, errors.Wrap(rows.Err(), "read migration ledger")
}

// readMigrations reads every *.sql file in fsys, in the lexical order they are
// applied in. The sort is written here rather than inherited from fs.ReadDir,
// because the order is the contract and belongs where a reader looks for it.
func readMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, errors.Wrap(err, "read migration directory")
	}
	var out []migration
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".sql" {
			continue
		}
		body, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, errors.Wrapf(err, "read migration %s", entry.Name())
		}
		sum := sha256.Sum256(body)
		out = append(out, migration{
			Name: entry.Name(),
			SQL:  string(body),
			Sum:  hex.EncodeToString(sum[:]),
		})
	}
	slices.SortFunc(out, func(a, b migration) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}
