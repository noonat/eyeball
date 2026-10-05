-- What happened to a comment after the reviewer wrote it: the agent's reply,
-- and whether the agent has acknowledged receiving it.
--
-- Each is TEXT NOT NULL with '' meaning absent, like every optional text column
-- in 0001. A reply nobody wrote and a reply that is empty are the same thing,
-- and a comment never delivered has no time it was delivered.
--
-- delivered is stamped when the agent's client says it printed the comment,
-- not when the daemon wrote it to a socket. A client killed between the two
-- would otherwise lose the comment, and delivering one twice costs a repeated
-- line.
ALTER TABLE comments ADD COLUMN reply TEXT NOT NULL DEFAULT '';
ALTER TABLE comments ADD COLUMN replied TEXT NOT NULL DEFAULT '';
ALTER TABLE comments ADD COLUMN delivered TEXT NOT NULL DEFAULT '';
