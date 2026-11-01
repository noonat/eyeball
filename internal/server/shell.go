package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball"
)

// buildHeader names the build a request was made for. The service worker sends
// it with each fetch that fills its cache. web/ts names it too.
const buildHeader = "Eyeball-Build"

// buildToken is a short hash of the daemon's build. The build string holds a
// space and an @, which the head would escape in a link and the worker would
// have to escape the same way for the cache to match. Hex is the same in both.
func (s *Server) buildToken() string {
	sum := sha256.Sum256([]byte(s.daemon.Build))
	return hex.EncodeToString(sum[:4])
}

// shellFiles lists what the worker keeps for the build with this token: the
// page, then every file the page links or app.css names. The head links the
// scripts and stylesheets with the token in the address, and the worker matches
// an address exactly, so the strings here are the head's.
func shellFiles(token string) []string {
	files := []string{
		"/",
		"/static/react.js?b=" + token,
		"/static/app.css?b=" + token,
		"/static/shell.css?b=" + token,
		"/manifest.webmanifest",
		"/static/icons/icon-180.png",
		"/static/icons/icon-192.png",
		"/static/icons/icon-512.png",
	}
	var fonts []string
	for name := range fontTags() {
		fonts = append(fonts, "/static/fonts/"+name)
	}
	slices.Sort(fonts)
	return append(files, fonts...)
}

// refuseOtherBuild answers 409 to a request made for another build, and
// reports whether it did. A worker fills its cache from the daemon that served
// its script. If the daemon restarted between the two, the files it would
// fetch are another build's, and a cache named for the old build would hold
// them. A request with no header is not made for a build.
func (s *Server) refuseOtherBuild(w http.ResponseWriter, r *http.Request) bool {
	want := r.Header.Get(buildHeader)
	if want == "" || want == s.buildToken() {
		return false
	}
	plain(w, http.StatusConflict, "This eyeball is another build than the one asked for.")
	return true
}

// handleWorker serves the service worker at the root, so its scope is the
// whole surface. The build's token and the list of files to keep are written in
// ahead of the script. The script's own stamp is taken before that, so it
// still hashes the worker's code alone, and a new build is a new worker to the
// browser, which installs it at its next update check.
func (s *Server) handleWorker(w http.ResponseWriter, _ *http.Request) error {
	js, err := eyeball.Worker()
	if err != nil {
		return err
	}
	token, err := json.Marshal(s.buildToken())
	if err != nil {
		return errors.WithStack(err)
	}
	files, err := json.Marshal(shellFiles(s.buildToken()))
	if err != nil {
		return errors.WithStack(err)
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write([]byte("var shellBuild=" + string(token) + ",shellFiles=" + string(files) + ";\n"))
	_, _ = w.Write(js)
	return nil
}
