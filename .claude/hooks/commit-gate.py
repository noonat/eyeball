#!/usr/bin/env python3
"""Refuse a git commit whose message has not been through the commit-message gate.

Two modes.

  record <file>   Note that the message in <file> passed. Run by the gate itself
                  as its last step, never by hand. A record that can be written
                  without a gate run is a record that will be, which happened
                  within five minutes of this file being written.
  check           A PreToolUse hook on Bash. Reads the tool call on stdin and
                  denies a git commit whose message has no record.

The record is the sha256 of the message bytes, stored as an empty file under
.git/eyeball-gate. Nothing is written into the message itself, so what lands in
git is exactly what was gated, and the repository's no-trailers rule is not
bent to carry a receipt. Change one character after gating and the hash changes,
so an edited message is an ungated message.

.git is local to a clone and never pushed, which is right for a record of what
was checked on this machine.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import shlex
import sys

# RECORDS is the directory under .git holding one empty file per gated message.
RECORDS = "eyeball-gate"

# KEEP is how many records to hold. A record matters between the gate run and
# the commit; the rest are kept only for a rebase that reuses a message.
KEEP = 50

# NOT_A_COMMIT and AMEND_REUSE are the two reasons a command needs no check.
NOT_A_COMMIT = "not a git commit"
AMEND_REUSE = "amend reusing an existing message"


def root() -> str:
    """The repository root, as the harness reports it."""
    return os.environ.get("CLAUDE_PROJECT_DIR") or os.getcwd()


def record_dir() -> str:
    """Where records live."""
    return os.path.join(root(), ".git", RECORDS)


def digest(message: str) -> str:
    """The key for one message: its bytes, with surrounding blank space ignored."""
    return hashlib.sha256(message.strip().encode()).hexdigest()


def segments(command: str) -> list[str]:
    """The command split on the shell operators that separate whole commands.

    A commit chained after something else is still a commit, and `make check &&
    git commit` is how every commit in this repository is made.
    """
    return re.split(r"&&|\|\||;|\|", command)


def commit_message(command: str) -> tuple[str | None, str]:
    """The message a git commit invocation would use.

    Returns the message, or None with a reason: either this is not a commit, or
    the message cannot be read from the command line.
    """
    for segment in segments(command):
        try:
            tokens = shlex.split(segment)
        except ValueError:
            continue
        if len(tokens) < 2 or os.path.basename(tokens[0]) != "git":
            continue
        if "commit" not in tokens[1:]:
            continue
        return message_from(tokens)
    return None, NOT_A_COMMIT


def message_from(tokens: list[str]) -> tuple[str | None, str]:
    """The message named by one git commit invocation's arguments.

    Several -m arguments are joined with a blank line between them, which is what
    git itself does with them.
    """
    parts: list[str | None] = []
    amend = False
    reuse = False
    i = 0
    while i < len(tokens):
        tok = tokens[i]
        if tok in ("-m", "--message"):
            i += 1
            if i < len(tokens):
                parts.append(tokens[i])
        elif tok.startswith("--message="):
            parts.append(tok.split("=", 1)[1])
        elif tok in ("-F", "--file"):
            i += 1
            if i < len(tokens):
                parts.append(read(tokens[i]))
        elif tok.startswith("--file="):
            parts.append(read(tok.split("=", 1)[1]))
        elif tok == "--amend":
            amend = True
        elif tok in ("--no-edit", "-C", "--reuse-message"):
            reuse = True
        i += 1
    if parts:
        return "\n\n".join(p for p in parts if p is not None), ""
    if amend and reuse:
        # The message is the one already on the commit, gated when it was
        # written. Nothing new to check.
        return None, AMEND_REUSE
    return None, "no message on the command line"


def read(path: str) -> str | None:
    """A file's contents, or None when it cannot be read."""
    try:
        with open(path) as handle:
            return handle.read()
    except OSError:
        return None


def deny(reason: str) -> int:
    """Refuse the tool call, telling the caller why."""
    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": reason,
        }
    }))
    return 0


def check() -> int:
    """Read one tool call from stdin and decide whether it may proceed."""
    try:
        call = json.loads(sys.stdin.read())
    except ValueError:
        return 0
    command: str = (call.get("tool_input") or {}).get("command") or ""
    message, why = commit_message(command)
    if message is None:
        if why in (NOT_A_COMMIT, AMEND_REUSE):
            return 0
        return deny(
            "This commit has no message on the command line, so the gate cannot "
            "read it. Write the message to a file, invoke the commit-message "
            "skill on it, then commit with -F.")
    if os.path.exists(os.path.join(record_dir(), digest(message))):
        return 0
    return deny(
        "This commit message has not been through the commit-message gate.\n"
        "Invoke the commit-message skill on it. The gate records its own pass as "
        "its last step, so there is nothing to run by hand and nothing to run in "
        "the wrong order.\n"
        "The record is a hash kept in .git, so nothing is added to the message "
        "itself. Editing the message after recording invalidates it, which is "
        "the point.")


def record(path: str) -> int:
    """Note that the message in a file passed the gate."""
    message = read(path)
    if message is None:
        print("cannot read %s" % path, file=sys.stderr)
        return 1
    os.makedirs(record_dir(), exist_ok=True)
    key = digest(message)
    open(os.path.join(record_dir(), key), "w").close()
    prune()
    print("recorded %s" % key[:12])
    return 0


def prune() -> None:
    """Drop the oldest records past KEEP."""
    directory = record_dir()
    names = [os.path.join(directory, n) for n in os.listdir(directory)]
    names.sort(key=os.path.getmtime, reverse=True)
    for stale in names[KEEP:]:
        try:
            os.remove(stale)
        except OSError:
            pass


def main() -> int:
    if len(sys.argv) > 1 and sys.argv[1] == "record":
        if len(sys.argv) != 3:
            print("usage: commit-gate.py record <message-file>", file=sys.stderr)
            return 1
        return record(sys.argv[2])
    return check()


if __name__ == "__main__":
    sys.exit(main())
