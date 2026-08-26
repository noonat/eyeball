-- Every table is STRICT, so a column rejects a value of the wrong type instead
-- of storing it. Without it an INTEGER column accepts 'hello', and the bug
-- surfaces wherever the row is read rather than where it was written.
--
-- Timestamps are TEXT in the layout internal/store documents: fixed width, UTC,
-- so a text sort is a chronological sort.
--
-- Optional text is NOT NULL with '' meaning absent. A note nobody wrote and a
-- note that is empty are the same thing here, and one representation means no
-- caller has to handle both.

CREATE TABLE projects (
    id      INTEGER PRIMARY KEY,
    root    TEXT NOT NULL UNIQUE,
    name    TEXT NOT NULL,
    created TEXT NOT NULL,
    -- The root is the identity: a project that moves is a new project, and
    -- re-registering it is free. The name is for display and repeats freely,
    -- because two checkouts can both be called web.
    CHECK (root <> ''),
    CHECK (name <> '')
) STRICT;

CREATE TABLE reviews (
    id         INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    agent      TEXT NOT NULL,
    title      TEXT NOT NULL,
    state      TEXT NOT NULL,
    created    TEXT NOT NULL,
    updated    TEXT NOT NULL,
    -- Waiting and working are not stored. They follow from whether the latest
    -- round has a decision, so there is one fact rather than two that can
    -- disagree. Abandonment has no round-level record, which is why the state
    -- is here at all.
    CHECK (state IN ('open', 'approved', 'abandoned')),
    CHECK (agent <> ''),
    CHECK (title <> '')
) STRICT;

CREATE TABLE review_paths (
    review_id INTEGER NOT NULL REFERENCES reviews (id) ON DELETE CASCADE,
    path      TEXT NOT NULL,
    -- The composite key is the uniqueness and the index at once, so an agent
    -- naming one path twice is refused by the schema.
    PRIMARY KEY (review_id, path),
    CHECK (path <> '')
) STRICT;

CREATE TABLE rounds (
    id          INTEGER PRIMARY KEY,
    review_id   INTEGER NOT NULL REFERENCES reviews (id) ON DELETE CASCADE,
    number      INTEGER NOT NULL,
    note        TEXT NOT NULL,
    base_commit TEXT NOT NULL,
    created     TEXT NOT NULL,
    -- A round is addressed by its number within its review, which is what the
    -- surface's routes spell.
    UNIQUE (review_id, number),
    CHECK (number >= 1),
    -- The note is the reviewer's brief and is required on every request. On a
    -- later round it says what was done about the last one.
    CHECK (note <> '')
    -- base_commit is '' outside git, where there is no record of a file before
    -- the edit and the first round shows whole files.
) STRICT;

CREATE TABLE round_files (
    round_id INTEGER NOT NULL REFERENCES rounds (id) ON DELETE CASCADE,
    path     TEXT NOT NULL,
    digest   TEXT NOT NULL,
    size     INTEGER NOT NULL,
    in_scope INTEGER NOT NULL,
    PRIMARY KEY (round_id, path),
    CHECK (path <> ''),
    CHECK (size >= 0),
    -- in_scope is frozen at capture rather than recomputed from the review's
    -- paths. The match is Go code, and a round should not change its answer
    -- when that code does.
    CHECK (in_scope IN (0, 1)),
    -- A path the agent deleted differs from the base and has no content, so it
    -- carries an empty digest. Leaving it out would make a deletion look like a
    -- file that never changed.
    CHECK (digest <> '' OR size = 0),
    -- Lowercase because the digest is what a path is built from, and two
    -- spellings are two paths on one filesystem and one path on another.
    CHECK (digest = '' OR (length(digest) = 64 AND digest = lower(digest)))
) STRICT;

CREATE TABLE comments (
    id         INTEGER PRIMARY KEY,
    round_id   INTEGER NOT NULL REFERENCES rounds (id) ON DELETE CASCADE,
    level      TEXT NOT NULL,
    path       TEXT,
    start_line INTEGER,
    end_line   INTEGER,
    quoted     TEXT,
    body       TEXT NOT NULL,
    created    TEXT NOT NULL,
    updated    TEXT NOT NULL,
    CHECK (level IN ('line', 'passage', 'file', 'review')),
    CHECK (body <> ''),
    -- A review-level comment is the one that says the approach is wrong. It has
    -- nowhere to point, and every other level names a path.
    CHECK ((level = 'review') = (path IS NULL)),
    CHECK (path IS NULL OR path <> ''),
    -- A line and a passage both carry a range: a passage in a rendered document
    -- maps to lines in the frozen source, because the source cannot move.
    CHECK ((level IN ('line', 'passage')) = (start_line IS NOT NULL)),
    CHECK ((start_line IS NULL) = (end_line IS NULL)),
    CHECK (start_line IS NULL OR start_line >= 1),
    CHECK (end_line IS NULL OR end_line >= start_line),
    -- The quoted text travels with the range, so an agent receives a path, a
    -- line range and the text without working out what was meant.
    CHECK ((start_line IS NOT NULL) = (quoted IS NOT NULL))
) STRICT;

CREATE TABLE decisions (
    -- One decision per round at most, which the primary key states exactly.
    -- Undecided is then a missing row rather than three columns that have to be
    -- NULL together.
    round_id INTEGER PRIMARY KEY REFERENCES rounds (id) ON DELETE CASCADE,
    verdict  TEXT NOT NULL,
    note     TEXT NOT NULL,
    decided  TEXT NOT NULL,
    -- Both verdicts release the round. Asking for changes finishes a reading as
    -- completely as approving does.
    CHECK (verdict IN ('approve', 'changes'))
) STRICT;
