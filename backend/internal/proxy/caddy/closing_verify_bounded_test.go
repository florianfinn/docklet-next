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

func TestVerifyTransientRoundEpochRemainsBounded(t *testing.T) {
	x := fixture(t)
	i := observationIntent(x, baseSite(), true, "edit-route")
	armed, reads := false, 0
	x.execFn = func([]string) hostio.ExecResult { armed = true; return httpResult(201, "changed") }
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if armed && strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			armed = false
			reads++
			http.Error(w, `{"message":"temporary daemon failure"}`, http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	waits := 0
	x.e.wait = func(ctx context.Context, d time.Duration) error {
		waits++
		require.LessOrEqual(t, waits, 51)
		x.now = x.now.Add(d)
		return ctx.Err()
	}
	got, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, got.State)
	require.NotEqual(t, dto.ContainerChanged, got.Reason)
	require.Positive(t, reads)
	require.Equal(t, reads, waits)
}
