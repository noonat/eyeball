-- Whether the agent has received a verdict, stamped by the same receipt that
-- stamps the comments it printed.
--
-- An approval with no comments had nothing to acknowledge, so once a wait that
-- printed it was killed and the agent's context cleared, nothing listed it any
-- more. eyeball status lists an ended review until its last verdict is
-- received, which needs this.
ALTER TABLE decisions ADD COLUMN delivered TEXT NOT NULL DEFAULT '';

-- A verdict given before this column existed was read, or not, with nothing to
-- record it. Calling every one of them received keeps an existing database from
-- listing every past approval as unread.
UPDATE decisions SET delivered = decided;
