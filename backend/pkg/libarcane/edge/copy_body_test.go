// Source: docklet-next/issues/386
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original forwarding copy outcome proofs for issue #386.

package edge

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

type failingWriter struct{ *httptest.ResponseRecorder }

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("client gone") }

// End of body stays success for JSON and event streams; only an upstream read error is reported.
func TestCopyBodyWithFlushOutcomes(t *testing.T) {
	for _, body := range []string{`{"success":true}`, "event: stats\ndata: {}\n\nevent: stats\ndata: {}\n\n"} {
		recorder := httptest.NewRecorder()
		require.NoError(t, CopyBodyWithFlush(recorder, iotest.OneByteReader(strings.NewReader(body))))
		require.Equal(t, body, recorder.Body.String())
		require.True(t, recorder.Flushed)
	}
	upstream := errors.New("upstream reset")
	recorder := httptest.NewRecorder()
	err := CopyBodyWithFlush(recorder, io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(upstream)))
	require.ErrorIs(t, err, upstream)
	require.Equal(t, "partial", recorder.Body.String())
	require.NoError(t, CopyBodyWithFlush(failingWriter{httptest.NewRecorder()}, strings.NewReader("data")))
}
