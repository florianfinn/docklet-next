// Source: getarcaneapp/arcane/backend/pkg/projects/compose_logs_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Verify structured service/TTY/stream metadata alongside text compatibility.

package projects

import (
	"bytes"
	"encoding/json"
	"go.getarcane.app/docker"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	composeLogsTestProjectInternal         = "projlogdemo"
	composeLogsTestNonTTYContainerInternal = "11112222333344445555666677778888"
	composeLogsTestTTYContainerInternal    = "9999aaaabbbbccccddddeeeeffff0000"
	composeLogsTestTimestampInternal       = "2024-08-26T10:00:00.000000000Z "
)

func TestComposeLogsMarksStderrWithoutChangingComposeOptions(t *testing.T) {
	logsQueries := make(chan string, 4)

	summary := func(id, service, image string) map[string]any {
		return map[string]any{
			"Id":     id,
			"Names":  []string{"/" + composeLogsTestProjectInternal + "-" + service + "-1"},
			"Image":  image,
			"State":  "running",
			"Status": "Up",
			"Labels": map[string]string{
				"com.docker.compose.project":          composeLogsTestProjectInternal,
				"com.docker.compose.service":          service,
				"com.docker.compose.container-number": "1",
				"com.docker.compose.oneoff":           "False",
			},
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")

		switch path {
		case "/_ping":
			w.Header().Set("Api-Version", "1.47")
			w.WriteHeader(http.StatusOK)
		case "/containers/json":
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]map[string]any{
				summary(composeLogsTestNonTTYContainerInternal, "web", "nginx"),
				summary(composeLogsTestTTYContainerInternal, "ttyd", "ttyd"),
			})) {
				return
			}
		case "/containers/" + composeLogsTestNonTTYContainerInternal + "/json":
			writeComposeInspectJSONInternal(t, w, composeLogsTestNonTTYContainerInternal, "web", false)
		case "/containers/" + composeLogsTestTTYContainerInternal + "/json":
			writeComposeInspectJSONInternal(t, w, composeLogsTestTTYContainerInternal, "ttyd", true)
		case "/containers/" + composeLogsTestNonTTYContainerInternal + "/logs":
			logsQueries <- r.URL.RawQuery
			w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
			_, err := w.Write(bytes.Join([][]byte{
				frameLogEntryInternal(stdcopy.Stdout, []byte(composeLogsTestTimestampInternal+"hello stdout\n")),
				frameLogEntryInternal(stdcopy.Stderr, []byte(composeLogsTestTimestampInternal+"hello stderr\n")),
			}, nil))
			if !assert.NoError(t, err) {
				return
			}
		case "/containers/" + composeLogsTestTTYContainerInternal + "/logs":
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			_, err := w.Write([]byte(composeLogsTestTimestampInternal + "raw tty output\n"))
			if !assert.NoError(t, err) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(server.URL, "http://"))

	out := &bytes.Buffer{}
	err := ComposeLogs(t.Context(), composeLogsTestProjectInternal, out, false, "all", "", true)
	require.NoError(t, err)

	body := out.String()
	require.Contains(t, body, "hello stdout")
	require.Contains(t, body, "| [STDERR] 2024-08-26T10:00:00.000000000Z hello stderr\n")
	require.Contains(t, body, "raw tty output")
	assert.NotContains(t, body, "[STDERR] raw tty")
	assert.NotContains(t, body, "[STDERR] hello stdout")

	query := <-logsQueries
	assert.Contains(t, query, "stdout=1")
	assert.Contains(t, query, "stderr=1")
	assert.Contains(t, query, "timestamps=1")
	assert.NotContains(t, query, "follow=")
	assert.NotContains(t, query, "tail=")
	var mu sync.Mutex
	var records []docker.LogRecord
	ctx := docker.WithLogSink(t.Context(), func(record docker.LogRecord) error {
		mu.Lock()
		defer mu.Unlock()
		records = append(records, record)
		return nil
	})
	out.Reset()
	require.NoError(t, ComposeLogs(ctx, composeLogsTestProjectInternal, out, false, "all", "", true))
	require.Empty(t, out.String(), "structured sources bypass the text pipe")
	require.Len(t, records, 3)
	observed := make(map[string]docker.LogRecord)
	for _, record := range records {
		observed[record.Stream] = record
	}
	require.Equal(t, "web", observed["stdout"].Service)
	require.Equal(t, "web", observed["stderr"].Service)
	require.Equal(t, "ttyd", observed["combined"].Service)
	require.Equal(t, composeLogsTestTimestampInternal+"hello stderr", observed["stderr"].Message)
}

func writeComposeInspectJSONInternal(t *testing.T, w http.ResponseWriter, containerID, serviceName string, tty bool) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"Id":      containerID,
		"Name":    "/" + composeLogsTestProjectInternal + "-" + serviceName + "-1",
		"Created": "2024-08-26T09:00:00Z",
		"Path":    "/bin/sh",
		"State": map[string]any{
			"Running":   true,
			"StartedAt": "2024-08-26T09:00:01Z",
		},
		"Config": map[string]any{
			"Tty":    tty,
			"Image":  "example-image",
			"Labels": map[string]string{"com.docker.compose.service": serviceName},
		},
		"HostConfig":      map[string]any{},
		"NetworkSettings": map[string]any{},
		"Mounts":          []any{},
	}))
}
