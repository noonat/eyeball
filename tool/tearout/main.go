// Command tearout builds Storybook and serves it on the tailnet beside the
// static design sheets, so they open on a real phone rather than in a desktop
// browser narrowed to 390 pixels.
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

// webDir holds the Node project, relative to the repository root.
const webDir = "web"

// storybookOutput is where `npm run build-storybook` writes, under webDir.
const storybookOutput = "storybook-static"

// rawDir is where tool/fixtures writes the files its views point at, under webDir.
// The raw route's addresses are absolute, so the stories ask the origin's root
// for them, not the path Storybook is served under.
const rawDir = "src/fixtures/_raw"

// storybookPath is where the built Storybook is served.
const storybookPath = "/storybook/"

// sheetStory is the full-screen address of the foundations sheet, relative to
// the Storybook it is served under.
const sheetStory = "iframe.html?id=foundations-sheet--states&viewMode=story"

// queueSheetStory is the full-screen address of the queue's sheet, relative to
// the Storybook it is served under.
const queueSheetStory = "iframe.html?id=queue-sheet--states&viewMode=story"

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

	built, err := buildStorybook(root)
	if err != nil {
		return err
	}

	host := tailnetAddr()
	if host == "" {
		host = allInterfaces
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	fmt.Printf("http://%s%s%s\n", addr, storybookPath, sheetStory)
	fmt.Printf("http://%s%s%s\n", addr, storybookPath, queueSheetStory)
	for _, page := range pages {
		fmt.Printf("http://%s/%s\n", addr, filepath.Base(page))
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	mux.Handle("/reviews/", http.FileServer(http.Dir(filepath.Join(root, webDir, rawDir))))
	mux.Handle(storybookPath, http.StripPrefix(storybookPath, http.FileServer(http.Dir(built))))
	server := &http.Server{
		Addr:              addr,
		Handler:           noStore(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		return errors.Wrapf(err, "serve %s", addr)
	}
	return nil
}

// buildStorybook builds Storybook and returns the directory it wrote. It is
// built on every start, since a served copy that predates the last edit to a
// component would show a state nobody can find in the source.
func buildStorybook(root string) (string, error) {
	web := filepath.Join(root, webDir)
	if _, err := os.Stat(filepath.Join(web, "node_modules")); err != nil {
		return "", errors.New("web/node_modules is missing. Run: make tools")
	}
	build := exec.Command("npm", "run", "--silent", "build-storybook")
	build.Dir = web
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return "", errors.Wrap(err, "build storybook")
	}
	return filepath.Join(web, storybookOutput), nil
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
