// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify counterexamples (C-B2/C-B4).

package caddy

import (
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
)

func TestVerifyRestartInitialTransientEpochUsesFirstSuccessfulRound(t *testing.T) {
	x := fixture(t)
	i := observationIntent(x, baseSite(), true, "edit-route")
	i.Restart = true
	x.c.State.StartedAt = nextEpoch
	failed := false
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if !failed && strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			failed = true
			http.Error(w, `{"message":"temporary daemon failure"}`, http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	x.execFn = func([]string) hostio.ExecResult { return httpResult(201, "changed") }
	got, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.True(t, failed)
	require.Equal(t, dto.Verified, got.State)
	require.Equal(t, dto.Routing, got.Evidence)
}
