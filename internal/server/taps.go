package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/noonat/eyeball"
)

// maxBeacon is the most a beacon may carry. The first one lists every open
// window, which a few hundred bytes each keeps well under it.
const maxBeacon = 8 << 10

// handleTap answers POST /taps: one beacon from the service worker's tap
// handler, written to the log as it arrived. The surface has no request log, so
// the route logs for itself. It lands beside the stamp of the sw.js served now,
// and a worker whose own stamp differs is a stale one.
//
// A beacon from a loopback page is refused for want of the token, which a
// worker cannot read from page storage. The phone uses the tailnet listener,
// which needs none.
func (s *Server) handleTap(w http.ResponseWriter, r *http.Request) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBeacon))
	var line bytes.Buffer
	if err != nil || json.Compact(&line, raw) != nil {
		plain(w, http.StatusBadRequest, "That is not a tap beacon.")
		return nil
	}
	served, err := eyeball.WorkerStamp()
	if err != nil {
		served = "unknown"
	}
	s.log.Info("tap", "beacon", line.String(), "served", served)
	w.WriteHeader(http.StatusNoContent)
	return nil
}
