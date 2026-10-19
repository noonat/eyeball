-- A project is the repository a working copy belongs to, and a working copy is
-- the directory a round was captured from: a main checkout, a linked worktree,
-- or a directory named by a marker file. Before this, every working copy was a
-- project of its own.
--
-- Each existing project becomes one working copy at its own root, with the same
-- id, and every round points at its review's project's copy. Whether a copy is
-- a main checkout, and which repository a project belongs to, needs git, so
-- both start unknown and the daemon fills them in.
CREATE TABLE working_copies (
    id         INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    root       TEXT NOT NULL UNIQUE,
    main       INTEGER NOT NULL DEFAULT 0 CHECK (main IN (0, 1)),
    created    TEXT NOT NULL,
    CHECK (root <> '')
) STRICT;

INSERT INTO working_copies (id, project_id, root, created)
    SELECT id, id, root, created FROM projects;

-- SQLite cannot add a UNIQUE column, so the uniqueness is a partial index over
-- the projects that have a repository.
ALTER TABLE projects ADD COLUMN repository TEXT CHECK (repository <> '');
CREATE UNIQUE INDEX projects_repository ON projects (repository) WHERE repository IS NOT NULL;

-- SQLite refuses a REFERENCES column with a non-null default, so the column is
-- added nullable, filled here, and kept filled by the triggers. Rebuilding
-- rounds to declare it NOT NULL would mean rebuilding every table that
-- references a round.
ALTER TABLE rounds ADD COLUMN working_copy_id INTEGER REFERENCES working_copies (id);
UPDATE rounds SET working_copy_id = (SELECT project_id FROM reviews WHERE reviews.id = rounds.review_id);

CREATE TRIGGER rounds_working_copy_on_insert BEFORE INSERT ON rounds
    WHEN NEW.working_copy_id IS NULL
BEGIN
    SELECT RAISE(ABORT, 'a round needs the working copy it was captured from');
END;

CREATE TRIGGER rounds_working_copy_on_update BEFORE UPDATE OF working_copy_id ON rounds
    WHEN NEW.working_copy_id IS NULL
BEGIN
    SELECT RAISE(ABORT, 'a round needs the working copy it was captured from');
END;
