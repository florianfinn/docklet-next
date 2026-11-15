// Source: docklet-next/issues/386
// Revision: dda7c807a3cdd6a46d2d3735c43f57447dedaa61
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original default exec boundary proofs for the streamed download (#386 review R1 B4, R2 zero-content cut, R3 owner cancellation after observed stdout).

package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
			_, _ = io.Copy(io.Discard, r.Body)
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

// zeroArchive is one regular file of zero bytes, so a cut inside its content ends in zero blocks.
func zeroArchive(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "zero.bin", Mode: 0o600, Size: 8192}))
	_, err := writer.Write(make([]byte, 8192))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

// Only a terminated exec with exit 0 whose output the tar reader follows to its end-of-archive blocks is a
// complete archive; Go's tar writer ends with exactly two zero blocks, like BusyBox tar.
func TestStreamedDownloadExecRequiresTerminatedCompleteArchive(t *testing.T) {
	previous := execExitWait
	execExitWait = 300 * time.Millisecond
	t.Cleanup(func() { execExitWait = previous })
	archive := sampleArchive(t)
	cut := archive[:len(archive)/2/512*512]
	zeros := zeroArchive(t)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "zero.bin"), make([]byte, 8192), 0o600))
	gnu, err := exec.CommandContext(t.Context(), "tar", "-cf", "-", "-C", root, ".").Output()
	require.NoError(t, err, "the GNU padding row runs the system tar")
	require.Zero(t, len(gnu)%10240, "GNU tar pads to its default record size")
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
		{"zero-content-complete", zeros, 0, 0, true},
		{"gnu-record-padding", gnu, 0, 0, true},
		{"zero-content-cut-then-zero", zeros[:2048], 1, 0, false},
		{"cut-before-end-blocks-then-zero", archive[:len(archive)-1024], 1, 0, false},
		{"one-end-block-then-zero", archive[:len(archive)-512], 1, 0, false},
		{"data-after-end-blocks", append(append([]byte{}, archive...), bytes.Repeat([]byte("x"), 512)...), 0, 0, false},
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

// signalWriter reports the first stdout bytes that reach the download.
type signalWriter struct {
	once     sync.Once
	received chan struct{}
}

func (w *signalWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.received) })
	return len(p), nil
}

// Owner cancellation after stdout reached the download closes the attach connection and joins the copy; the
// attach is caller-owned in the pinned moby client.
func TestStreamedDownloadExecOwnerCancellationClosesAttach(t *testing.T) {
	release, closed := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	stopServer := func() { releaseOnce.Do(func() { close(release) }) }
	defer stopServer()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/helper/exec"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"Id":"tar-exec"}`)
		case strings.HasSuffix(r.URL.Path, "/exec/tar-exec/start"):
			_, _ = io.Copy(io.Discard, r.Body)
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			_, _ = fmt.Fprint(rw, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
			frame := make([]byte, 8)
			frame[0] = 1
			binary.BigEndian.PutUint32(frame[4:], 512)
			_, _ = rw.Write(frame)
			_, _ = rw.Write(zeroArchive(t)[:512])
			_ = rw.Flush()
			readDone := make(chan struct{})
			go func() {
				_, _ = rw.ReadByte()
				close(closed)
				close(readDone)
			}()
			select {
			case <-readDone:
			case <-release:
			}
		default:
			t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(server.URL, "http://")), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stdout := &signalWriter{received: make(chan struct{})}
	d := &StreamingDownload{s: &Service{}, ctx: ctx, cli: cli}
	done := make(chan error, 1)
	go func() {
		done <- d.execStream("helper", []string{"tar", "-cf", "-", "-C", "/scratch/data", "."}, stdout)
	}()
	select {
	case <-stdout.received:
	case <-time.After(2 * time.Second):
		stopServer()
		t.Fatal("stdout did not reach the download")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Error("owner cancellation did not close the active attach and join the copy")
		stopServer()
		<-done
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Error("attach connection was not closed")
	}
}
