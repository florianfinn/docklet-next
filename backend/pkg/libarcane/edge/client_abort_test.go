// Source: docklet-next/issues/330
// Revision: a37d77d05c92bde3a4eab0cf6fa47ab36411e011
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original regression for aborted local handlers on tunneled requests (M16 acceptance DV1 D).

package edge

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// An aborted handler gets no response, as with net/http, instead of crashing the agent; other panics still propagate
// (class rows A5, A8).
func TestServeLocalTreatsAbortHandlerAsNoResponse(t *testing.T) {
	aborting := &TunnelClient{handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })}
	require.True(t, aborting.serveLocalInternal(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)))

	serving := &TunnelClient{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })}
	rec := httptest.NewRecorder()
	require.False(t, serving.serveLocalInternal(rec, httptest.NewRequest(http.MethodGet, "/", nil)))
	require.Equal(t, http.StatusNoContent, rec.Code)

	failure := errors.New("example failure")
	failing := &TunnelClient{handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(failure) })}
	require.PanicsWithValue(t, failure, func() {
		failing.serveLocalInternal(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}
