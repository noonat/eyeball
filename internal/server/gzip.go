package server

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// gzipLevel is the level the daemon compresses at. Measured on a long review's
// rows, level 6 took them from 4.8 MB to 258 KB.
const gzipLevel = 6

// compressed wraps w so a response is gzipped when the request accepts it. The
// returned function ends the stream and must be called once the handler has
// returned. Vary is set whether or not the request accepts gzip, since a cache
// holding the identity bytes has to know they depend on the header.
func compressed(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, func()) {
	w.Header().Add("Vary", "Accept-Encoding")
	if !acceptsGzip(r) {
		return w, func() {}
	}
	gz := &gzipWriter{ResponseWriter: w}
	return gz, gz.finish
}

// acceptsGzip says whether the request lists gzip with a quality above zero.
func acceptsGzip(r *http.Request) bool {
	for _, field := range r.Header.Values("Accept-Encoding") {
		for _, item := range strings.Split(field, ",") {
			name, params, _ := strings.Cut(strings.TrimSpace(item), ";")
			if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
				continue
			}
			return strings.TrimSpace(params) != "q=0" && strings.TrimSpace(params) != "q=0.0"
		}
	}
	return false
}

// gzipWriter compresses what a handler writes. It decides on the first write or
// status whether the response has a body to compress, so a 304 or a 204 passes
// through with no Content-Encoding.
type gzipWriter struct {
	http.ResponseWriter
	zip     *gzip.Writer
	decided bool
}

// decide starts compression for a status that carries a body.
func (g *gzipWriter) decide(status int) {
	if g.decided {
		return
	}
	g.decided = true
	bodyless := status < 200 || status == http.StatusNoContent || status == http.StatusNotModified
	if bodyless || g.Header().Get("Content-Encoding") != "" {
		return
	}
	g.Header().Set("Content-Encoding", "gzip")
	// The length of the identity bytes would be wrong for what is sent.
	g.Header().Del("Content-Length")
	g.zip, _ = gzip.NewWriterLevel(g.ResponseWriter, gzipLevel)
}

// WriteHeader passes the status on once compression is decided.
func (g *gzipWriter) WriteHeader(status int) {
	g.decide(status)
	g.ResponseWriter.WriteHeader(status)
}

// Write compresses p.
func (g *gzipWriter) Write(p []byte) (int, error) {
	g.decide(http.StatusOK)
	if g.zip == nil {
		return g.ResponseWriter.Write(p)
	}
	return g.zip.Write(p)
}

// Unwrap lets http.ResponseController reach the writer underneath.
func (g *gzipWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

// finish flushes the compressed stream's trailer.
func (g *gzipWriter) finish() {
	if g.zip != nil {
		_ = g.zip.Close()
	}
}
