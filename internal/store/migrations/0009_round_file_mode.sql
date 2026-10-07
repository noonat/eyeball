-- The git mode a round captured each file with: 100644 for a regular file,
-- 100755 for an executable one, 120000 for a symlink, and '' for a deletion and
-- for every round taken before this column existed. eyeball check compares it
-- with the commit a branch moves to, since a file approved as text and merged
-- as executable, or as a symlink with the same bytes, is not what was
-- approved. The end-of-branch review of 012 found both passing when only bytes
-- were compared.
ALTER TABLE round_files ADD COLUMN mode TEXT NOT NULL DEFAULT '';
