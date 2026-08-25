#!/usr/bin/env python3
"""Serve the tearouts on the tailnet so they open on the real phone.

A design that looks right in a desktop browser at 390px and wrong in the hand
has taught nothing, and this is a phone-first surface.

Cache-Control: no-store is the whole reason this exists rather than
`python3 -m http.server`. That sends only Last-Modified, and iOS Safari will
show a stale copy of a tearout just edited, which reads as "the change did not
work" rather than as a cache.

Run: python3 docs/design/serve.py [port]
"""

import http.server
import json
import os
import subprocess
import sys


class Handler(http.server.SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header("Cache-Control", "no-store, must-revalidate")
        super().end_headers()

    def log_message(self, fmt, *args):
        sys.stderr.write("%s %s\n" % (self.address_string(), fmt % args))


def tailnet_ip():
    try:
        out = subprocess.run(
            ["tailscale", "status", "--json"], capture_output=True, timeout=5
        ).stdout
        ips = json.loads(out)["Self"]["TailscaleIPs"]
        return ips[0] if ips else ""
    except Exception:
        return ""


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8110
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    host = tailnet_ip() or "0.0.0.0"
    server = http.server.ThreadingHTTPServer((host, port), Handler)
    print("http://%s:%d/foundations.html" % (host, port))
    print("http://%s:%d/queue.html" % (host, port))
    print("http://%s:%d/review.html" % (host, port))
    server.serve_forever()


if __name__ == "__main__":
    main()
