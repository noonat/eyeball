// Command tearout serves the design tearouts on the tailnet, so they open on a
// real phone rather than in a desktop browser narrowed to 390 pixels.
//
// Cache-Control: no-store is the reason this exists rather than any static file
// server. A server that sends only Last-Modified leaves iOS Safari showing a
// stale copy of a page just edited, which reads as the change not having worked
// rather than as a cache.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
)

// designDir holds the tearouts, relative to the repository root.
const designDir = "docs/design"

// defaultPort is the port served when the command line names none.
const defaultPort = 8110

// allInterfaces is the address served when there is no tailnet address to bind.
const allInterfaces = "0.0.0.0"

// status is the part of `tailscale status --json` this reads.
type status struct {
	Self struct {
		TailscaleIPs []string
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "tearout: %v\n", err)
		os.Exit(1)
	}
}

// run serves until interrupted, or reports why it could not start.
func run(args []string) error {
	port := defaultPort
	if len(args) > 0 {
		parsed, err := strconv.Atoi(args[0])
		if err != nil {
			return errors.Wrapf(err, "port %q", args[0])
		}
		port = parsed
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, designDir)
	pages, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		return errors.Wrapf(err, "list %s", dir)
	}
	sort.Strings(pages)

	host := tailnetAddr()
	if host == "" {
		host = allInterfaces
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	for _, page := range pages {
		fmt.Printf("http://%s/%s\n", addr, filepath.Base(page))
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           noStore(http.FileServer(http.Dir(dir))),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		return errors.Wrapf(err, "serve %s", addr)
	}
	return nil
}

// noStore wraps a handler so no response is ever cached.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		next.ServeHTTP(w, r)
	})
}

// tailnetAddr is this machine's first tailnet address, or empty when there is
// none and the caller should fall back to every interface.
func tailnetAddr() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return ""
	}
	var s status
	if err := json.Unmarshal(out, &s); err != nil {
		return ""
	}
	if len(s.Self.TailscaleIPs) == 0 {
		return ""
	}
	return s.Self.TailscaleIPs[0]
}

// repoRoot walks up from the working directory for the module root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", errors.WithStack(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above the working directory")
		}
		dir = parent
	}
}
