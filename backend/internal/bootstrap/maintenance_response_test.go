// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regression for committed responses under hub maintenance (M16 review CFR1-3).

package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaintenanceResponseKeepsCommittedResponse(t *testing.T) {
	for _, commit := range []string{"header", "write", "flush"} {
		t.Run(commit, func(t *testing.T) {
			rec := httptest.NewRecorder()
			var cancelled atomic.Bool
			w := &maintenanceResponseInternal{ResponseWriter: rec, cancelled: &cancelled}
			switch commit {
			case "header":
				w.WriteHeader(http.StatusOK)
			case "write":
				_, _ = w.Write([]byte("prefix"))
			case "flush":
				w.Flush()
			}
			before := rec.Body.String()
			cancelled.Store(true)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("suffix"))
			require.Equal(t, []any{http.StatusOK, before + "suffix"}, []any{rec.Code, rec.Body.String()})
		})
	}
}
