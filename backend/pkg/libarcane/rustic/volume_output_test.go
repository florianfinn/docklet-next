// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original pre-allocation Docker frame budget proofs.

package rustic

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
)

func dockerFrame(stream byte, body string) []byte {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(body)))
	return append(header, body...)
}

type headerOnlyReader struct {
	header      *bytes.Reader
	payloadRead bool
}

func (r *headerOnlyReader) Read(p []byte) (int, error) {
	if r.header.Len() == 0 {
		r.payloadRead = true
		return 0, io.ErrUnexpectedEOF
	}
	return r.header.Read(p)
}
func TestVolumeOutputRejectsFrameLengthBeforePayloadRead(t *testing.T) {
	header := make([]byte, 8)
	header[0] = 1
	binary.BigEndian.PutUint32(header[4:], ^uint32(0))
	source := &headerOnlyReader{header: bytes.NewReader(header)}
	output, diagnostics, err := readVolumeOutput(source)
	require.Empty(t, output)
	require.Empty(t, diagnostics)
	require.Error(t, err)
	require.False(t, source.payloadRead, "oversized frame payload must never be read or allocated")
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "volume.backup.engine-output-invalid", problem.Code)
}
func TestVolumeOutputBudgetIncludesBothStreamsAndRefusesTruncation(t *testing.T) {
	body := strings.Repeat("a", dto.MaxVolumeResponseBytes)
	output, diagnostics, err := readVolumeOutput(bytes.NewReader(dockerFrame(1, body)))
	require.NoError(t, err)
	require.Len(t, output, dto.MaxVolumeResponseBytes)
	require.Empty(t, diagnostics)
	for _, framed := range [][]byte{
		append(dockerFrame(1, body), dockerFrame(2, "x")...),
		append(dockerFrame(2, body), dockerFrame(1, "x")...),
		dockerFrame(1, "truncated")[:10],
		{1, 0},
		dockerFrame(3, "daemon-error"),
	} {
		output, diagnostics, err := readVolumeOutput(bytes.NewReader(framed))
		require.Error(t, err)
		require.Empty(t, output, "never return truncated stdout as success")
		require.Empty(t, diagnostics)
	}
	output, diagnostics, err = readVolumeOutput(bytes.NewReader(append(dockerFrame(1, "ok"), dockerFrame(2, "warning")...)))
	require.NoError(t, err)
	require.Equal(t, "ok", output)
	require.Equal(t, "warning", diagnostics)
}
