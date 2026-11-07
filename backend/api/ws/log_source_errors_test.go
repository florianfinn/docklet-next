// Source: docklet-next/issues/177
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original daemon error classification proofs for log sources against a real Docker client.

package ws

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func TestLogSourceDaemonErrorCategories(t *testing.T) {
	for _, tc := range []struct {
		status   int
		category string
	}{
		{404, "not-found"}, {403, "permission"}, {500, "internal"}, {503, "unavailable"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Contains(t, r.URL.Path, "/containers/example/logs")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"message":"synthetic daemon failure"}`))
			}))
			defer daemon.Close()
			docker, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.49"))
			require.NoError(t, err)
			defer docker.Close()
			_, err = docker.ContainerLogs(t.Context(), "example", client.ContainerLogsOptions{ShowStdout: true})
			require.Error(t, err)
			require.Equal(t, tc.category, logSourceErrorCategory(err))
		})
	}
}
