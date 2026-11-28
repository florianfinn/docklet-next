// Source: docklet-next/issues/693
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original window-end test through the real project service, details and Compose chain.

package project

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The window end of a project log read reaches the Docker logs query through
// ProjectService.StreamProjectLogs, the details service and the real Compose SDK.
func TestProjectLogsPassWindowEndToDocker(t *testing.T) {
	queries := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
		switch path {
		case "/_ping":
			w.Header().Set("Api-Version", "1.47")
			_, _ = io.WriteString(w, "OK")
		case "/version":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.47", "MinAPIVersion": "1.24", "Version": "27.0.0"})
		case "/containers/json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{{"Id": "app-id", "Names": []string{"/alpha-app-1"}, "Image": "registry.example.com/app:1", "State": "running", "Status": "Up", "Labels": map[string]string{"com.docker.compose.project": "alpha", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.oneoff": "False"}}})
		case "/containers/app-id/json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "app-id", "Name": "/alpha-app-1", "Created": "2026-01-02T03:00:00Z", "Path": "/bin/sh", "State": map[string]any{"Running": true, "StartedAt": "2026-01-02T03:00:01Z"}, "Config": map[string]any{"Tty": true, "Image": "registry.example.com/app:1", "Labels": map[string]string{"com.docker.compose.service": "app"}}, "HostConfig": map[string]any{}, "NetworkSettings": map[string]any{}, "Mounts": []any{}})
		case "/containers/app-id/logs":
			queries <- r.URL.RawQuery
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			_, _ = io.WriteString(w, "2026-01-02T03:04:05.000000000Z example line\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() { server.CloseClientConnections(); server.Close() })
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	lines := make(chan string, 16)
	require.NoError(t, svc.StreamProjectLogs(ctx, row.ID, lines, false, "all", "2026-01-02T03:00:00Z", "2026-01-02T03:05:00Z", true))
	select {
	case query := <-queries:
		require.Contains(t, query, "until=1767323100", "the window end reaches the Docker logs query")
		require.Contains(t, query, "since=1767322800")
	default:
		t.Fatal("the Compose chain never read the container log")
	}
}
