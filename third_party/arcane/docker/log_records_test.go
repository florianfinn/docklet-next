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
