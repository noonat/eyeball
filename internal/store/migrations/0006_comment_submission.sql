-- Two changes to comments, which SQLite can only make by rebuilding the table,
-- since a CHECK constraint cannot be altered.
--
-- submission is the key the page draws when a comment is first sent and sends
-- again on every retry. A retry after a timeout cannot tell whether the first
-- send was stored, and a key already held stores nothing new. '' means none,
-- like every optional text column, and only non-empty keys are unique. A table
-- of recent keys in memory was the alternative, and a retry after a restart
-- would then store the comment twice.
--
-- A file-level comment may now carry quoted text with no range. A tap on a
-- removed line opens the file's box with that line quoted: the line has no
-- number in the round's frozen file, and putting the quote into the body would
-- give it to the agent as the reviewer's own words.
CREATE TABLE comments_new (
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
    reply      TEXT NOT NULL DEFAULT '',
    replied    TEXT NOT NULL DEFAULT '',
    delivered  TEXT NOT NULL DEFAULT '',
    submission TEXT NOT NULL DEFAULT '',
    CHECK (level IN ('line', 'passage', 'file', 'review')),
    CHECK (body <> ''),
    CHECK ((level = 'review') = (path IS NULL)),
    CHECK (path IS NULL OR path <> ''),
    CHECK ((level IN ('line', 'passage')) = (start_line IS NOT NULL)),
    CHECK ((start_line IS NULL) = (end_line IS NULL)),
    CHECK (start_line IS NULL OR start_line >= 1),
    CHECK (end_line IS NULL OR end_line >= start_line),
    -- A range always carries its text. Text with no range is a removed line,
    -- quoted on its file.
    CHECK (start_line IS NULL OR quoted IS NOT NULL),
    CHECK (quoted IS NULL OR start_line IS NOT NULL OR level = 'file')
);

INSERT INTO comments_new
    (id, round_id, level, path, start_line, end_line, quoted, body, created,
     updated, reply, replied, delivered)
SELECT id, round_id, level, path, start_line, end_line, quoted, body, created,
       updated, reply, replied, delivered
FROM comments;

DROP TABLE comments;
ALTER TABLE comments_new RENAME TO comments;

CREATE UNIQUE INDEX comments_submission ON comments (submission)
    WHERE submission <> '';
