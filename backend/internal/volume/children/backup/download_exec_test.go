// Source: docklet-next/issues/386
// Revision: dda7c807a3cdd6a46d2d3735c43f57447dedaa61
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original default exec boundary proofs for the streamed download (#386 review R1 B4).

package backup

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// execDocker answers exec create, a real upgraded attach carrying stdout frames, and exec inspects that report
// running for the first `running` calls and then the exit code.
func execDocker(t *testing.T, stdout []byte, running int32, exitCode int) *client.Client {
	t.Helper()
	var inspects atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/helper/exec"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"Id":"tar-exec"}`)
		case strings.HasSuffix(r.URL.Path, "/exec/tar-exec/start"):
			conn, rw, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			defer conn.Close()
			_, _ = fmt.Fprint(rw, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
			for start := 0; start < len(stdout); start += 4096 {
				chunk := stdout[start:min(start+4096, len(stdout))]
				header := make([]byte, 8)
				header[0] = 1
				binary.BigEndian.PutUint32(header[4:], uint32(len(chunk)))
				_, _ = rw.Write(header)
				_, _ = rw.Write(chunk)
			}
			_ = rw.Flush()
		case strings.HasSuffix(r.URL.Path, "/exec/tar-exec/json"):
			w.Header().Set("Content-Type", "application/json")
			still := inspects.Add(1) <= running
			_, _ = fmt.Fprintf(w, `{"ID":"tar-exec","Running":%t,"ExitCode":%d}`, still, exitCode)
		default:
			t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(server.URL, "http://")), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

// Only a terminated exec with exit 0 whose output ends with the tar end-of-archive blocks is a complete archive.
func TestStreamedDownloadExecRequiresTerminatedCompleteArchive(t *testing.T) {
	previous := execExitWait
	execExitWait = 300 * time.Millisecond
	t.Cleanup(func() { execExitWait = previous })
	archive := sampleArchive(t)
	cut := archive[:len(archive)/2/512*512]
	for _, row := range []struct {
		name     string
		stdout   []byte
		running  int32
		exitCode int
		ok       bool
	}{
		{"terminated-zero", archive, 0, 0, true},
		{"exit-race-then-zero", archive, 2, 0, true},
		{"terminated-nonzero", archive, 0, 2, false},
		{"output-cut-still-running", cut, 1000, 0, false},
		{"complete-output-still-running", archive, 1000, 0, false},
		{"output-cut-then-zero", cut, 1, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			d := &StreamingDownload{s: &Service{}, ctx: t.Context(), cli: execDocker(t, row.stdout, row.running, row.exitCode)}
			var out bytes.Buffer
			err := d.execStream("helper", []string{"tar", "-cf", "-", "-C", "/scratch/data", "."}, &out)
			require.Equal(t, row.stdout, out.Bytes())
			if row.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err, "an incomplete or failed producer must not finish the archive")
			}
		})
	}
}
