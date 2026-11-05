// Source: docklet-next/issues/341
// Revision: 01f87a4c35d7a054ec346b16c1ba62fd5581b27a
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Direct agent requests follow the proxy deadline, not the shared client timeout.
package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A Direct agent answer that arrives after the shared client's timeout but
// within the proxy deadline must reach the caller.
func TestDirectProxyOutlastsSharedClientTimeout(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"status":"up_to_date"}}`))
	}))
	t.Cleanup(agent.Close)
	shared := &http.Client{Timeout: 100 * time.Millisecond}
	s := New(nil, shared, nil, nil)
	var out struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, s.JSON(t.Context(), &Target{id: "example", targetURL: agent.URL}, http.MethodPost, "/api/environments/0/updater/projects/stack/check", nil, &out))
	require.Equal(t, "up_to_date", out.Data.Status)
	require.Equal(t, 100*time.Millisecond, shared.Timeout, "the shared client keeps its own timeout")
}

// Without a hard client timeout, the caller's deadline still ends the request.
func TestDirectProxyHonoursCallerDeadline(t *testing.T) {
	release := make(chan struct{})
	agent := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); agent.Close() })
	s := New(nil, &http.Client{}, nil, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, e := s.Execute(ctx, &Target{id: "example", targetURL: agent.URL}, http.MethodGet, "/api/health", nil)
	require.True(t, errors.Is(e, context.DeadlineExceeded))
	require.Less(t, time.Since(started), 5*time.Second)
}
