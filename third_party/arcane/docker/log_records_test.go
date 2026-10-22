// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original bounded allocation and source metadata regression tests.

package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

type repeatingLogByte struct{}

func (repeatingLogByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'z'
	}
	return len(p), nil
}
func generatedLogFrame(size int64) io.Reader {
	var h [8]byte
	h[0] = 2
	binary.BigEndian.PutUint32(h[4:], uint32(size))
	return io.MultiReader(bytes.NewReader(h[:]), io.LimitReader(repeatingLogByte{}, size))
}
func TestLogProducerBoundedAllocation(t *testing.T) {
	for _, follow := range []bool{false, true} {
		t.Run(map[bool]string{true: "follow", false: "snapshot"}[follow], func(t *testing.T) {
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			var got []LogRecord
			ctx := WithLogSink(t.Context(), func(r LogRecord) error { got = append(got, r); return nil })
			err := StreamContainerLogs(ctx, io.NopCloser(generatedLogFrame(32<<20)), nil, follow, false)
			runtime.ReadMemStats(&after)
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.LessOrEqual(t, len(got[0].Message), MaxLogLineBytes)
			require.True(t, got[0].Truncated)
			require.Equal(t, "stderr", got[0].Stream)
			allocated := after.TotalAlloc - before.TotalAlloc
			t.Logf("generated input=%d retained=%d allocated=%d", 32<<20, len(got[0].Message), allocated)
			require.Less(t, allocated, uint64(2<<20), "allocation must not scale with the input frame")
		})
	}
}
func TestLogProducerUTF8AndDiscard(t *testing.T) {
	for _, ending := range []string{"", "\nnext\n"} {
		input := strings.Repeat("界", MaxLogLineBytes) + ending
		var got []LogRecord
		ctx := WithLogSink(t.Context(), func(r LogRecord) error { got = append(got, r); return nil })
		require.NoError(t, StreamContainerLogs(ctx, io.NopCloser(strings.NewReader(input)), nil, true, true))
		require.True(t, got[0].Truncated)
		require.True(t, utf8.ValidString(got[0].Message))
		require.Equal(t, "combined", got[0].Stream)
		require.LessOrEqual(t, len(got[0].Message), MaxLogLineBytes)
		if ending != "" {
			require.Len(t, got, 2)
			require.Equal(t, "next", got[1].Message)
		} else {
			require.Len(t, got, 1)
		}
	}
}
func TestLogProducerPreservesStreamAndApplicationText(t *testing.T) {
	var frames bytes.Buffer
	writeDockerLogFrameInternal(t, &frames, 1, "  [STDERR] application text  \npartial")
	writeDockerLogFrameInternal(t, &frames, 2, "diagnostic\n")
	writeDockerLogFrameInternal(t, &frames, 1, " end\n")
	var got []LogRecord
	ctx := WithLogSink(t.Context(), func(r LogRecord) error { got = append(got, r); return nil })
	require.NoError(t, StreamContainerLogs(ctx, io.NopCloser(&frames), nil, false, false))
	require.Equal(t, []LogRecord{{Message: "  [STDERR] application text  ", Stream: "stdout"}, {Message: "diagnostic", Stream: "stderr"}, {Message: "partial end", Stream: "stdout"}}, got)
}
func TestLogProducerSinkFailureStopsSource(t *testing.T) {
	ctx := WithLogSink(t.Context(), func(LogRecord) error { return context.Canceled })
	require.ErrorIs(t, StreamContainerLogs(ctx, io.NopCloser(strings.NewReader("line\nsecond\n")), nil, true, true), context.Canceled)
}

func TestLogProducerBoundsMultibytePrefixBeforeConcatenation(t *testing.T) {
	prefix := strings.Repeat("界", 2<<20)
	output := make(chan string, 1)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	require.NoError(t, ReadLogLines(t.Context(), strings.NewReader("message\n"), output, prefix))
	runtime.ReadMemStats(&after)
	line := <-output
	require.LessOrEqual(t, len(line), MaxLogLineBytes)
	require.True(t, utf8.ValidString(line))
	require.True(t, strings.HasSuffix(line, LogTruncatedMarker))
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(2<<20), "prefix concatenation must not copy the full prefix")
}

func TestLogProducerDaemonSystemError(t *testing.T) {
	for _, text := range []string{"daemon cannot read logs", strings.Repeat("界", MaxLogLineBytes)} {
		var frame bytes.Buffer
		writeDockerLogFrameInternal(t, &frame, 3, text)
		emitted := 0
		ctx := WithLogSink(t.Context(), func(LogRecord) error { emitted++; return nil })
		err := StreamContainerLogs(ctx, io.NopCloser(&frame), nil, true, false)
		require.ErrorContains(t, err, "Docker daemon log error: ")
		require.Zero(t, emitted)
		require.True(t, utf8.ValidString(err.Error()))
		require.LessOrEqual(t, len(err.Error()), 1024+len("Docker daemon log error: "))
		if len(text) > 1024 {
			require.Contains(t, err.Error(), LogTruncatedMarker)
		} else {
			require.ErrorContains(t, err, text)
		}
	}
}

func TestLogProducerDaemonSystemErrorHugeFrameIsNotDrained(t *testing.T) {
	var header [8]byte
	header[0] = 3
	binary.BigEndian.PutUint32(header[4:], 1<<30)
	reader := &limitedDiagnosticReader{reader: io.MultiReader(bytes.NewReader(header[:]), repeatingLogByte{})}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := NewLogReader(reader, false).Next(t.Context())
	runtime.ReadMemStats(&after)
	require.ErrorContains(t, err, "Docker daemon log error: ")
	require.Contains(t, err.Error(), LogTruncatedMarker)
	require.LessOrEqual(t, reader.read, 4096+8, "must stop after the diagnostic prefix, including bounded read-ahead")
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(64<<10))
}

type limitedDiagnosticReader struct {
	reader io.Reader
	read   int
}

func (r *limitedDiagnosticReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += n
	return n, err
}

func TestLogProducerDaemonSystemErrorTruncatedAndCancelled(t *testing.T) {
	var header [8]byte
	header[0] = 3
	binary.BigEndian.PutUint32(header[4:], 20)
	for _, payload := range []string{"partial", ""} {
		reader := io.MultiReader(bytes.NewReader(header[:]), strings.NewReader(payload))
		err := StreamContainerLogs(t.Context(), io.NopCloser(reader), nil, true, false)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		require.ErrorContains(t, err, payload)
	}

	input, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- StreamContainerLogs(ctx, input, nil, true, false) }()
	// Pipe.Write returns after the decoder consumes the header; cancellation now
	// interrupts the blocked system diagnostic read through the reader closer.
	_, err := writer.Write(header[:])
	require.NoError(t, err)
	cancel()
	select {
	case err = <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled diagnostic read did not stop")
	}
}
