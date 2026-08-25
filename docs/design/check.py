#!/usr/bin/env python3
"""Hold the tearouts to the properties a browser will not complain about.

A class in the markup that no stylesheet defines renders as unstyled text and
nobody notices, because the page still looks about right. Same for a fragment
link pointing at an id that does not exist: expand silently does nothing.

An agent name written into prose is a second copy of an identifier the page
already displays, and the two drift. So prose may not contain one at all.

Icons get their own checks. A glyph is selected by an `i-<name>` class whose
mask lives in app.css, so a span carrying `i` with no `i-` class draws nothing,
and one carrying text draws that text next to the glyph. Both look plausible
until somebody opens the page in an engine that renders them.

Run: python3 docs/design/check.py
"""

import glob
import os
import re
import sys
from html.parser import HTMLParser

HERE = os.path.dirname(os.path.abspath(__file__))
VOID = {"meta", "link", "br", "img", "hr", "input", "source", "col"}


def stylesheets():
    css = ""
    for sheet in ("app.css", "tearout.css"):
        with open(os.path.join(HERE, sheet)) as handle:
            css += re.sub(r"/\*.*?\*/", "", handle.read(), flags=re.S)
    return css


class Scan(HTMLParser):
    def __init__(self):
        super().__init__()
        self.classes = set()
        self.ids = set()
        self.frags = set()
        self.stack = []
        self.problems = []
        self.icon_depth = None

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        classes = (a.get("class") or "").split()
        self.classes |= set(classes)
        if a.get("id"):
            self.ids.add(a["id"])
        href = a.get("href") or ""
        if href.startswith("#"):
            self.frags.add(href[1:])
        if "i" in classes and not any(c.startswith("i-") for c in classes):
            self.problems.append(
                'class "i" with no i-<name> beside it draws nothing: %s'
                % " ".join(classes))
        if "i" in classes and self.icon_depth is None:
            self.icon_depth = len(self.stack)
        if tag not in VOID:
            self.stack.append(tag)

    def handle_endtag(self, tag):
        if tag in VOID:
            return
        if not self.stack:
            self.problems.append("stray </%s>" % tag)
            return
        if self.stack[-1] != tag:
            self.problems.append("</%s> closes <%s>" % (tag, self.stack[-1]))
        self.stack.pop()
        if self.icon_depth is not None and len(self.stack) <= self.icon_depth:
            self.icon_depth = None

    def handle_data(self, data):
        if self.icon_depth is not None and data.strip():
            self.problems.append(
                "text inside an icon span: %r. The glyph comes from the i-<name> "
                "class, so this renders beside it." % data.strip()[:30])


def main():
    css = stylesheets()
    known = set(re.findall(r"\.([A-Za-z_][\w-]*)", css))
    pages = sorted(glob.glob(os.path.join(HERE, "*.html")))
    problems = 0
    for path in pages:
        scan = Scan()
        with open(path) as handle:
            scan.feed(handle.read())
        name = os.path.basename(path)
        for cls in sorted(scan.classes - known):
            scan.problems.append("class .%s is used but no stylesheet defines it" % cls)
        for frag in sorted(scan.frags - scan.ids):
            scan.problems.append("href #%s points at no id on the page" % frag)
        for left in scan.stack:
            scan.problems.append("<%s> never closed" % left)
        for problem in scan.problems:
            print("%s: %s" % (name, problem))
            problems += 1

    # An agent is displayed inside a .agent span. The same name appearing in a
    # sentence is a copy that nothing keeps in step, and renaming the agent
    # leaves the sentence behind.
    agents = set()
    for path in pages:
        with open(path) as handle:
            agents |= set(re.findall(r'<span class="agent">([^<]+)</span>', handle.read()))
    agents = {a for a in agents if not a.startswith("~")}
    for path in pages:
        with open(path) as handle:
            body = handle.read()
        body = re.sub(r'<span class="agent">[^<]*</span>', "", body)
        text = re.sub(r"<[^>]+>", " ", body)
        for agent in sorted(agents):
            if agent in text:
                print("%s: prose names the agent %s, which the page also displays"
                      % (os.path.basename(path), agent))
                problems += 1

    # app.css holds a generated block. A generator that takes too much with it
    # deletes hand-written rules, and the page still renders: unstyled, but
    # rendered. These are the declarations nothing else can stand in for.
    # Anchored at the start of a line: a bare substring check passes on the
    # wrong rule, because ".ic .i {" contains ".i {".
    load_bearing = [
        r"--prose:", r"--mono:", r"--t-md:", r"--pad:", r"--gap:", r"--r-md:",
        r"--ink:", r"--bg:", r"--add:", r"--del:",
        r"\.i \{", r"\.app \{", r"\.page \{", r"\.row \{", r"\.decide \{",
    ]
    for decl in load_bearing:
        if not re.search(r"(?m)^\s*" + decl, css):
            print("app.css no longer declares %s" % decl.replace("\\", ""))
            problems += 1

    # The mask table and icons.txt have to agree, or a class resolves to nothing
    # and leaves a gap that reads as a spacing bug.
    listed = set(open(os.path.join(HERE, "icons.txt")).read().strip().split(","))
    mapped = set(re.findall(r"\.i-([a-z0-9-]+) \{\s*--icon:", css))
    for name in sorted(listed - mapped):
        print("icons.txt lists %s but app.css has no mask for it" % name)
        problems += 1
    for name in sorted(mapped - listed):
        print("app.css maps %s but icons.txt does not list it" % name)
        problems += 1

    print("checked %d pages, %d problems" % (len(pages), problems))
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
