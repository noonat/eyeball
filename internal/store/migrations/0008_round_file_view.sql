-- The view a file opens in at a round, as the agent chose it in the request,
-- and '' for the file's own default. It is per round rather than per review,
-- because each request says what that round is asking, and a choice carried
-- into the next round would outlive its question.
ALTER TABLE round_files ADD COLUMN view TEXT NOT NULL DEFAULT '';
