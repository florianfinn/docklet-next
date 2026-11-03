// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original M16 answer for requests cancelled by hub maintenance or agent shutdown.

package bootstrap

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// maintenanceResponseInternal turns the server failure of a request that hub maintenance cancelled into the same
// 503 maintenance problem that new requests receive; other responses, and any response whose status was already
// committed by WriteHeader, Write or Flush, pass through unchanged. A non-nil answer writes a different replacement.
type maintenanceResponseInternal struct {
	http.ResponseWriter
	cancelled *atomic.Bool
	answer    func(http.ResponseWriter)
	committed bool
	replaced  bool
}

func (w *maintenanceResponseInternal) WriteHeader(code int) {
	if w.committed {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.committed = code >= http.StatusOK || code == http.StatusSwitchingProtocols
	if code >= http.StatusInternalServerError && w.cancelled.Load() {
		w.replaced = true
		if w.answer != nil {
			w.answer(w.ResponseWriter)
			return
		}
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
	w.committed = true
	return w.ResponseWriter.Write(p)
}

func (w *maintenanceResponseInternal) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok && !w.replaced {
		w.committed = true
		f.Flush()
	}
}

func (w *maintenanceResponseInternal) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// agentShutdownResponseInternal answers a server failure of an agent request that shutdown cancelled as the agent being
// unavailable, the code the hub serves cached inventory for or reports as host unavailable.
func agentShutdownResponseInternal(stopping *atomic.Bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&maintenanceResponseInternal{ResponseWriter: w, cancelled: stopping, answer: agentUnavailableInternal}, r)
	})
}

func agentUnavailableInternal(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Del("Content-Length")
	w.WriteHeader(http.StatusBadGateway)
	_, _ = w.Write([]byte(`{"success":false,"code":"agent.unavailable"}`))
}
