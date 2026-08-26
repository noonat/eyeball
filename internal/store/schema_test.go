package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_schemaAcceptsAWholeReview(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	ids := seed(g, t, s)
	at := "2026-08-26T04:05:06.000Z"

	// A file the agent changed inside the review, one outside it for context,
	// and one it deleted, which carries no digest.
	files := []struct {
		path    string
		digest  string
		size    int64
		inScope int
	}{
		{path: "docs/product.md", digest: digest('a'), size: 120, inScope: 1},
		{path: "internal/store/store.go", digest: digest('b'), size: 4000, inScope: 0},
		{path: "docs/gone.md", digest: "", size: 0, inScope: 1},
	}
	insertFile := "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (?, ?, ?, ?, ?)"
	for _, f := range files {
		exec(g, t, s, insertFile, ids.round, f.path, f.digest, f.size, f.inScope)
	}

	// One comment at each of the four levels.
	comments := []struct {
		level  string
		path   any
		start  any
		end    any
		quoted any
	}{
		{level: "line", path: "docs/product.md", start: 12, end: 12, quoted: "one line"},
		{level: "passage", path: "docs/product.md", start: 20, end: 24, quoted: "a passage"},
		{level: "file", path: "docs/product.md", start: nil, end: nil, quoted: nil},
		{level: "review", path: nil, start: nil, end: nil, quoted: nil},
	}
	insertComment := "INSERT INTO comments (round_id, level, path, start_line, end_line, quoted, body, created, updated) VALUES (?, ?, ?, ?, ?, ?, 'a point', :at, :at)"
	for _, c := range comments {
		exec(g, t, s, insertComment, ids.round, c.level, c.path, c.start, c.end, c.quoted, sql.Named("at", at))
	}

	exec(g, t, s, "INSERT INTO decisions (round_id, verdict, note, decided) VALUES (?, 'changes', '', '2026-08-26T04:05:07.000Z')", ids.round)

	var fileCount, commentCount int
	row := s.db.QueryRowContext(t.Context(), "SELECT (SELECT count(*) FROM round_files), (SELECT count(*) FROM comments)")
	g.Expect(row.Scan(&fileCount, &commentCount)).To(Succeed())
	g.Expect(fileCount).To(Equal(3))
	g.Expect(commentCount).To(Equal(4))
}

func Test_schemaCascadesFromTheProject(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	ids := seed(g, t, s)
	exec(g, t, s, "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (?, 'a.md', ?, 1, 1)", ids.round, digest('a'))
	exec(g, t, s, "INSERT INTO comments (round_id, level, path, body, created, updated) VALUES (?, 'file', 'a.md', 'a point', '2026-08-26T04:05:06.000Z', '2026-08-26T04:05:06.000Z')", ids.round)
	exec(g, t, s, "INSERT INTO decisions (round_id, verdict, note, decided) VALUES (?, 'approve', '', '2026-08-26T04:05:07.000Z')", ids.round)

	// Deleting the project takes everything under it, so the pruning nothing
	// does yet stays one statement when it arrives.
	exec(g, t, s, "DELETE FROM projects WHERE id = ?", ids.project)

	tables := []string{"reviews", "review_paths", "rounds", "round_files", "comments", "decisions"}
	for _, table := range tables {
		t.Run(table, func(t *testing.T) {
			g := NewWithT(t)
			var count int
			row := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table)
			g.Expect(row.Scan(&count)).To(Succeed())
			g.Expect(count).To(BeZero())
		})
	}
}

func Test_schemaRejectionsAreNotVacuous(t *testing.T) {
	g := NewWithT(t)
	s := openStore(g, t)
	ids := seed(g, t, s)

	// Every row in Test_schemaRejectsBadRows is handed all six named arguments,
	// and most statements reference two or three. This is the same argument set
	// on a row that must be accepted, so a driver that started refusing unused
	// named arguments would fail here rather than turning all of those
	// rejections into passes for the wrong reason.
	_, err := s.db.ExecContext(
		t.Context(),
		"INSERT INTO comments (round_id, level, path, body, created, updated) VALUES (:round, 'file', 'ok.md', 'a point', :at, :at)",
		sql.Named("project", ids.project),
		sql.Named("review", ids.review),
		sql.Named("round", ids.round),
		sql.Named("at", "2026-08-26T04:05:06.000Z"),
		sql.Named("digest", digest('c')),
		sql.Named("upper", digest('A')),
	)
	g.Expect(err).NotTo(HaveOccurred())
}

func Test_schemaRejectsBadRows(t *testing.T) {
	// One row per CHECK and per unique constraint. A constraint nobody has
	// watched refuse a row is not a constraint.
	rows := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "project with no root",
			sql:  "INSERT INTO projects (root, name, created) VALUES ('', 'web', :at)",
			want: "CHECK",
		},
		{
			name: "project with no name",
			sql:  "INSERT INTO projects (root, name, created) VALUES ('/tmp/other', '', :at)",
			want: "CHECK",
		},
		{
			name: "project root reused",
			sql:  "INSERT INTO projects (root, name, created) VALUES ('/tmp/project', 'again', :at)",
			want: "UNIQUE",
		},
		{
			name: "review with no agent",
			sql:  "INSERT INTO reviews (project_id, agent, title, state, created, updated) VALUES (:project, '', 'a title', 'open', :at, :at)",
			want: "CHECK",
		},
		{
			name: "review with no title",
			sql:  "INSERT INTO reviews (project_id, agent, title, state, created, updated) VALUES (:project, 'agent-1', '', 'open', :at, :at)",
			want: "CHECK",
		},
		{
			name: "review in a state that is not one of the three",
			sql:  "INSERT INTO reviews (project_id, agent, title, state, created, updated) VALUES (:project, 'agent-1', 'a title', 'waiting', :at, :at)",
			want: "CHECK",
		},
		{
			name: "review under a project that does not exist",
			sql:  "INSERT INTO reviews (project_id, agent, title, state, created, updated) VALUES (9999, 'agent-1', 'a title', 'open', :at, :at)",
			want: "FOREIGN KEY",
		},
		{
			name: "claimed path that is empty",
			sql:  "INSERT INTO review_paths (review_id, path) VALUES (:review, '')",
			want: "CHECK",
		},
		{
			name: "claimed path repeated in one review",
			sql:  "INSERT INTO review_paths (review_id, path) VALUES (:review, 'docs')",
			want: "UNIQUE",
		},
		{
			name: "round numbered below one",
			sql:  "INSERT INTO rounds (review_id, number, note, base_commit, created) VALUES (:review, 0, 'look at this', '', :at)",
			want: "CHECK",
		},
		{
			name: "round with no note",
			sql:  "INSERT INTO rounds (review_id, number, note, base_commit, created) VALUES (:review, 2, '', '', :at)",
			want: "CHECK",
		},
		{
			name: "round number repeated in one review",
			sql:  "INSERT INTO rounds (review_id, number, note, base_commit, created) VALUES (:review, 1, 'look at this', '', :at)",
			want: "UNIQUE",
		},
		{
			name: "captured file with no path",
			sql:  "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (:round, '', :digest, 1, 1)",
			want: "CHECK",
		},
		{
			name: "captured file whose size is text",
			sql:  "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (:round, 'a.md', :digest, 'big', 1)",
			want: "cannot store TEXT value in INTEGER column",
		},
		{
			name: "captured file of negative size",
			sql:  "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (:round, 'a.md', :digest, -1, 1)",
			want: "CHECK",
		},
		{
			name: "captured file neither in scope nor out of it",
			sql:  "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (:round, 'a.md', :digest, 1, 2)",
			want: "CHECK",
		},
		{
			name: "digest of the wrong length",
			sql:  "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (:round, 'a.md', 'abc', 1, 1)",
			want: "CHECK",
		},
		{
			name: "digest in uppercase",
			sql:  "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (:round, 'a.md', :upper, 1, 1)",
			want: "CHECK",
		},
		{
			name: "deleted file with a size",
			sql:  "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (:round, 'a.md', '', 12, 1)",
			want: "CHECK",
		},
		{
			name: "captured path repeated in one round",
			sql:  "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (:round, 'kept.md', :digest, 1, 1)",
			want: "UNIQUE",
		},
		{
			name: "comment at a level that does not exist",
			sql:  "INSERT INTO comments (round_id, level, path, body, created, updated) VALUES (:round, 'hunk', 'a.md', 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "comment with no body",
			sql:  "INSERT INTO comments (round_id, level, path, body, created, updated) VALUES (:round, 'file', 'a.md', '', :at, :at)",
			want: "CHECK",
		},
		{
			name: "review-level comment that names a path",
			sql:  "INSERT INTO comments (round_id, level, path, body, created, updated) VALUES (:round, 'review', 'a.md', 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "file-level comment that names no path",
			sql:  "INSERT INTO comments (round_id, level, path, body, created, updated) VALUES (:round, 'file', NULL, 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "comment with an empty path",
			sql:  "INSERT INTO comments (round_id, level, path, body, created, updated) VALUES (:round, 'file', '', 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "line comment with no range",
			sql:  "INSERT INTO comments (round_id, level, path, body, created, updated) VALUES (:round, 'line', 'a.md', 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "file comment carrying a range",
			sql:  "INSERT INTO comments (round_id, level, path, start_line, end_line, quoted, body, created, updated) VALUES (:round, 'file', 'a.md', 1, 1, 'x', 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "range with a start and no end",
			sql:  "INSERT INTO comments (round_id, level, path, start_line, quoted, body, created, updated) VALUES (:round, 'line', 'a.md', 1, 'x', 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "range starting below line one",
			sql:  "INSERT INTO comments (round_id, level, path, start_line, end_line, quoted, body, created, updated) VALUES (:round, 'line', 'a.md', 0, 0, 'x', 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "range ending before it starts",
			sql:  "INSERT INTO comments (round_id, level, path, start_line, end_line, quoted, body, created, updated) VALUES (:round, 'line', 'a.md', 9, 4, 'x', 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "range carrying no quoted text",
			sql:  "INSERT INTO comments (round_id, level, path, start_line, end_line, body, created, updated) VALUES (:round, 'line', 'a.md', 1, 1, 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "quoted text with no range",
			sql:  "INSERT INTO comments (round_id, level, path, quoted, body, created, updated) VALUES (:round, 'file', 'a.md', 'x', 'a point', :at, :at)",
			want: "CHECK",
		},
		{
			name: "verdict that is neither approve nor changes",
			sql:  "INSERT INTO decisions (round_id, verdict, note, decided) VALUES (:round, 'maybe', '', :at)",
			want: "CHECK",
		},
		{
			name: "second decision on one round",
			sql:  "INSERT INTO decisions (round_id, verdict, note, decided) VALUES (:round, 'approve', '', :at)",
			want: "UNIQUE",
		},
	}
	for _, c := range rows {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			s := openStore(g, t)
			ids := seed(g, t, s)
			// The seeded graph already holds a claimed path, a round, a
			// captured file and a decision, so the rows testing a repeat have
			// something to collide with.
			exec(g, t, s, "INSERT INTO round_files (round_id, path, digest, size, in_scope) VALUES (?, 'kept.md', ?, 1, 1)", ids.round, digest('a'))
			exec(g, t, s, "INSERT INTO decisions (round_id, verdict, note, decided) VALUES (?, 'changes', '', '2026-08-26T04:05:07.000Z')", ids.round)

			_, err := s.db.ExecContext(
				t.Context(),
				c.sql,
				sql.Named("project", ids.project),
				sql.Named("review", ids.review),
				sql.Named("round", ids.round),
				sql.Named("at", "2026-08-26T04:05:06.000Z"),
				sql.Named("digest", digest('c')),
				sql.Named("upper", digest('A')),
			)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(c.want))
		})
	}
}

// seeded holds the ids of the graph every schema test starts from.
type seeded struct {
	// Project, Review and Round are the rows a bad row is attached to.
	project, review, round int64
}

// digest is a 64-character digest built from one repeated character, which is
// enough to satisfy the schema without pretending to be a real hash.
func digest(c byte) string {
	out := make([]byte, 64)
	for i := range out {
		out[i] = c
	}
	return string(out)
}

// exec runs one statement and fails the test if it does not succeed.
func exec(g *WithT, t *testing.T, s *Store, query string, args ...any) {
	g.THelper()
	_, err := s.db.ExecContext(t.Context(), query, args...)
	g.Expect(err).NotTo(HaveOccurred())
}

// openStore opens a migrated store on a fresh database.
func openStore(g *WithT, t *testing.T) *Store {
	g.THelper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "eyeball.db"))
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// seed inserts one project, one review with a claimed path, and its first
// round, which is the smallest graph a comment or a captured file can hang off.
func seed(g *WithT, t *testing.T, s *Store) seeded {
	g.THelper()
	at := "2026-08-26T04:05:06.000Z"
	var ids seeded

	res, err := s.db.ExecContext(t.Context(), "INSERT INTO projects (root, name, created) VALUES ('/tmp/project', 'project', ?)", at)
	g.Expect(err).NotTo(HaveOccurred())
	ids.project, err = res.LastInsertId()
	g.Expect(err).NotTo(HaveOccurred())

	res, err = s.db.ExecContext(t.Context(), "INSERT INTO reviews (project_id, agent, title, state, created, updated) VALUES (?, 'agent-1', 'a title', 'open', ?, ?)", ids.project, at, at)
	g.Expect(err).NotTo(HaveOccurred())
	ids.review, err = res.LastInsertId()
	g.Expect(err).NotTo(HaveOccurred())

	exec(g, t, s, "INSERT INTO review_paths (review_id, path) VALUES (?, 'docs')", ids.review)

	res, err = s.db.ExecContext(t.Context(), "INSERT INTO rounds (review_id, number, note, base_commit, created) VALUES (?, 1, 'look at this', '', ?)", ids.review, at)
	g.Expect(err).NotTo(HaveOccurred())
	ids.round, err = res.LastInsertId()
	g.Expect(err).NotTo(HaveOccurred())

	return ids
}
