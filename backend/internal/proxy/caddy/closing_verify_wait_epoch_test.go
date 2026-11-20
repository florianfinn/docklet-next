// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify counterexamples (C-B2/C-B4).

package caddy

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestVerifyTransientEpochWaitDeadlineRetainsLastReason(t *testing.T) {
	x := fixture(t)
	i := observationIntent(x, baseSite(), true, "edit-route")
	x.e.activated.Store(i.Request.ID, x.now.Add(-6800*time.Millisecond))
	armed, failed := false, false
	afterProbeReads := 0
	x.execFn = func([]string) hostio.ExecResult { armed = true; afterProbeReads = 0; return httpResult(201, "changed") }
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if armed && !failed && strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			afterProbeReads++
			if afterProbeReads == 1 {
				return false
			}
			failed = true
			http.Error(w, `{"message":"temporary daemon failure"}`, http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	x.e.wait = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }
	got, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.True(t, failed)
	require.Equal(t, dto.AppliedUnverified, got.State)
	require.Equal(t, dto.NoObservableDifference, got.Reason)
}
