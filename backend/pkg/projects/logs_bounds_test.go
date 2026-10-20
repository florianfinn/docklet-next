// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original generated-input Compose allocation and writer bounds.

package projects

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/docker"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type repeatedComposeByte struct{}

func (repeatedComposeByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}
func TestComposeLogSourceBoundedAllocation(t *testing.T) {
	for _, tty := range []bool{false, true} {
		t.Run(map[bool]string{false: "framed", true: "tty"}[tty], func(t *testing.T) {
			var header [8]byte
			header[0] = 2
			binary.BigEndian.PutUint32(header[4:], 32<<20)
			var input io.Reader = io.LimitReader(repeatedComposeByte{}, 32<<20)
			if !tty {
				input = io.MultiReader(bytes.NewReader(header[:]), input)
			}
			upstream := io.NopCloser(input)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			stream := &stderrMarkedDemuxedStreamInternal{upstream: upstream, records: docker.NewLogReader(upstream, tty), ctx: t.Context(), tty: tty}
			// The only output retained by the test is the already bounded single line.
			output, err := io.ReadAll(stream)
			runtime.ReadMemStats(&after)
			require.NoError(t, err)
			require.LessOrEqual(t, len(output), docker.MaxLogLineBytes+9)
			require.Contains(t, string(output), docker.LogTruncatedMarker)
			allocated := after.TotalAlloc - before.TotalAlloc
			t.Logf("generated=%d output=%d allocated=%d", 32<<20, len(output), allocated)
			require.Less(t, allocated, uint64(2<<20))
		})
	}
}

type measuredLogWriter struct{ writes []string }

func (w *measuredLogWriter) Write(b []byte) (int, error) {
	w.writes = append(w.writes, string(b))
	return len(b), nil
}
func TestComposeLogWritersSerializeAndBoundPrefixes(t *testing.T) {
	output := &measuredLogWriter{}
	consumer := &writerConsumer{out: output}
	var group sync.WaitGroup
	for _, service := range []string{"web", "worker"} {
		group.Go(func() {
			consumer.Log(service, "first")
			consumer.Err(service, strings.Repeat("界", docker.MaxLogLineBytes))
		})
	}
	group.Wait()
	require.Len(t, output.writes, 4)
	for _, line := range output.writes {
		require.LessOrEqual(t, len(line), docker.MaxLogLineBytes)
		require.True(t, strings.HasSuffix(line, "\n"))
		require.True(t, strings.HasPrefix(line, "web | ") || strings.HasPrefix(line, "worker | "))
	}
	consumer.Log(strings.Repeat("界", docker.MaxLogLineBytes), "last")
	require.LessOrEqual(t, len(output.writes[4]), docker.MaxLogLineBytes)
}
