// Source: getarcaneapp/arcane/backend/pkg/projects/logs_demux_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Verify discard-to-newline and bounded TTY/single-stream wrapping.

package projects

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/docker"
)

func frameLogEntryInternal(fd stdcopy.StdType, payload []byte) []byte {
	header := make([]byte, 8)
	header[0] = byte(fd)
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	return append(header, payload...)
}

func decodeDemuxedStreamInternal(t *testing.T, stream io.Reader) string {
	t.Helper()
	decoded := &bytes.Buffer{}
	sinkErr := &bytes.Buffer{}
	_, err := stdcopy.StdCopy(decoded, sinkErr, stream)
	require.NoError(t, err)
	require.Empty(t, sinkErr.String(), "every entry must be re-framed as stdout")
	return decoded.String()
}

func TestDemultiplexedLogStreamWithStderrMarking(t *testing.T) {
	longRun := strings.Repeat("x", maxLogEntryBytesInternal+100)
	source := bytes.Join([][]byte{
		frameLogEntryInternal(stdcopy.Stdout, []byte("line-one\npar")),
		frameLogEntryInternal(stdcopy.Stderr, []byte("boom\n")),
		frameLogEntryInternal(stdcopy.Stdout, []byte("tial\n")),
		frameLogEntryInternal(stdcopy.Stderr, []byte(longRun)),
		frameLogEntryInternal(stdcopy.Stderr, []byte("\nfinal")),
	}, nil)

	resp := io.NopCloser(bytes.NewReader(source))
	stream := &stderrMarkedDemuxedStreamInternal{upstream: resp, records: docker.NewLogReader(resp, false), ctx: t.Context()}

	body := decodeDemuxedStreamInternal(t, stream)
	lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	require.Len(t, lines, 5)
	require.Equal(t, []string{"line-one", "[STDERR] boom", "partial"}, lines[:3])
	require.LessOrEqual(t, len(lines[3]), docker.MaxLogLineBytes)
	require.True(t, strings.HasSuffix(lines[3], docker.LogTruncatedMarker))
	require.Equal(t, "[STDERR] final", lines[4])
}

type fakeResponseBodyInternal struct {
	io.Reader
	closed bool
}

func (b *fakeResponseBodyInternal) Close() error {
	b.closed = true
	return nil
}

type stubLogAPIClientInternal struct {
	client.APIClient

	resp      io.ReadCloser
	logsCalls int
	inspect   client.ContainerInspectResult
}

func (s *stubLogAPIClientInternal) ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	s.logsCalls++
	return s.resp, nil
}

func (s *stubLogAPIClientInternal) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return s.inspect, nil
}

func TestLogsDemuxClientContainerLogsDecisionInternal(t *testing.T) {
	muxed := bytes.Join([][]byte{
		frameLogEntryInternal(stdcopy.Stdout, []byte("out\n")),
		frameLogEntryInternal(stdcopy.Stderr, []byte("err\n")),
	}, nil)

	tests := []struct {
		name           string
		ttyRecorded    bool
		containerKnown bool
		showStdout     bool
		showStderr     bool
		wantWrapped    bool
	}{
		{name: "non-tty streams are transformed", containerKnown: true, showStdout: true, showStderr: true, wantWrapped: true},
		{name: "tty bounded", ttyRecorded: true, containerKnown: true, showStdout: true, showStderr: true, wantWrapped: true},
		{name: "unknown container fails before opening logs", containerKnown: false, showStdout: true, showStderr: true, wantWrapped: false},
		{name: "single-stream request bounded", containerKnown: true, showStdout: true, showStderr: false, wantWrapped: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := muxed
			if tt.ttyRecorded {
				source = []byte("combined\n")
			}
			body := &fakeResponseBodyInternal{Reader: bytes.NewReader(source)}
			api := &stubLogAPIClientInternal{resp: body}
			inner := &logsDemuxDockerClientInternal{APIClient: api}
			if tt.containerKnown {
				inner.recordContainerTTYInternal(" CTR-1 ", tt.ttyRecorded)
			}

			got, err := inner.ContainerLogs(t.Context(), "ctr-1", client.ContainerLogsOptions{ShowStdout: tt.showStdout, ShowStderr: tt.showStderr})
			if !tt.wantWrapped {
				require.ErrorContains(t, err, "inspect required")
				require.Nil(t, got)
				require.Zero(t, api.logsCalls)
				return
			}

			require.NoError(t, err)
			if tt.ttyRecorded {
				b, err := io.ReadAll(got)
				require.NoError(t, err)
				require.Equal(t, "combined\n", string(b))
			} else {
				require.Equal(t, "out\n[STDERR] err\n", decodeDemuxedStreamInternal(t, got))
			}
			require.NoError(t, got.Close())
			require.True(t, body.closed, "closing the wrapped stream must close the upstream response")
		})
	}
}

func TestLogsDemuxNilInspectConfigInvalidatesMetadata(t *testing.T) {
	api := &stubLogAPIClientInternal{}
	inner := &logsDemuxDockerClientInternal{APIClient: api}
	api.inspect.Container.Config = &containertypes.Config{Tty: true}
	_, err := inner.ContainerInspect(t.Context(), "example", client.ContainerInspectOptions{})
	require.NoError(t, err)
	_, known := inner.containerTTYInternal("example")
	require.True(t, known)
	api.inspect.Container.Config = nil
	_, err = inner.ContainerInspect(t.Context(), "example", client.ContainerInspectOptions{})
	require.ErrorContains(t, err, "missing inspect config")
	_, known = inner.containerTTYInternal("example")
	require.False(t, known)
	got, err := inner.ContainerLogs(t.Context(), "example", client.ContainerLogsOptions{})
	require.ErrorContains(t, err, "inspect required")
	require.Nil(t, got)
	require.Zero(t, api.logsCalls)
}
