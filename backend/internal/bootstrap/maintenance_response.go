// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original M16 answer for requests cancelled by hub maintenance.

package bootstrap

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// maintenanceResponseInternal turns the server failure of a request that hub maintenance cancelled into the same
// 503 maintenance problem that new requests receive; other responses pass through unchanged.
type maintenanceResponseInternal struct {
	http.ResponseWriter
	cancelled *atomic.Bool
	replaced  bool
}

func (w *maintenanceResponseInternal) WriteHeader(code int) {
	if code >= http.StatusInternalServerError && w.cancelled.Load() {
		w.replaced = true
		w.ResponseWriter.Header().Set("Content-Type", "application/problem+json")
		w.ResponseWriter.Header().Del("Content-Length")
		w.ResponseWriter.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w.ResponseWriter).Encode(maintenanceProblemInternal())
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *maintenanceResponseInternal) Write(p []byte) (int, error) {
	if w.replaced {
		return len(p), nil
	}
	return w.ResponseWriter.Write(p)
}

func (w *maintenanceResponseInternal) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok && !w.replaced {
		f.Flush()
	}
}

func (w *maintenanceResponseInternal) Unwrap() http.ResponseWriter { return w.ResponseWriter }
