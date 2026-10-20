// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestRuntimeWriterBoundsUnterminatedAndLargeBatches(t *testing.T) {
	a := &runtimeAppenderInternal{}
	w, err := NewRuntimeWriter(t.Context(), a, "example-run", "")
	require.NoError(t, err)
	for range 128 {
		_, err = w.Write([]byte(strings.Repeat("x", 64<<10)))
		require.NoError(t, err)
	}
	w.mu.Lock()
	buffered := len(w.buffer)
	w.mu.Unlock()
	require.LessOrEqual(t, buffered, runtimeCaptureLineLimit)
	_, err = w.Write([]byte("\nnext\n"))
	require.NoError(t, err)
	_, err = w.Write([]byte(strings.Repeat("line\n", 2048)))
	require.NoError(t, err)
	require.NoError(t, w.Finish(t.Context()))
	messages, batches := a.recorded()
	require.Len(t, messages, 2050)
	require.True(t, messages[0].OutputTruncated)
	require.Contains(t, messages[0].Output.Text, "truncated")
	require.Equal(t, "next", messages[1].Output.Text)
	for _, message := range messages {
		require.LessOrEqual(t, len(message.Output.Text), runtimeCaptureLineLimit)
		require.True(t, utf8.ValidString(message.Output.Text))
	}
	for _, size := range batches {
		require.LessOrEqual(t, size, 32)
	}
}
