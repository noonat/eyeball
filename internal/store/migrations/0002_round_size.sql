-- A round's size, as the queue and the review page show it: files touched and
-- lines added and removed. It is measured against the previous round rather
-- than against the base, because that is the change the round opens on, and it
-- is stored because the alternative is diffing every waiting review on every
-- queue render.
--
-- These are totals over the paths inside the review's scope. They are not a sum
-- over round_files: a file changed in one round and reverted in the next moves
-- without appearing in the later capture, and per-file rows would have nowhere
-- to put it.
-- There is no backfill. A round written before this migration reads back as no
-- files and no lines, because the numbers were never computed and cannot be
-- recovered from what was stored. Nothing has shipped, so the answer for an
-- existing database is to delete it. The runner's own refusal does not cover
-- this: it fires on a migration whose bytes changed, not on one that adds a
-- column with no history behind it.
ALTER TABLE rounds ADD COLUMN files_changed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE rounds ADD COLUMN lines_added INTEGER NOT NULL DEFAULT 0;
ALTER TABLE rounds ADD COLUMN lines_removed INTEGER NOT NULL DEFAULT 0;
