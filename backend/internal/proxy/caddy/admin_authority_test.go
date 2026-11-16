// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original admin authority and observation-window tests from the M27 early smoke.

package caddy

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestAdminAuthorityIsTheListenAddress(t *testing.T) {
	for address, want := range map[string]string{
		"localhost:2019": "localhost:2019", ":2019": "localhost:2019", "0.0.0.0:2019": "localhost:2019",
		"localhost": "localhost:2019", "127.0.0.1:2020": "127.0.0.1:2020", "[::1]:2019": "[::1]:2019", "[::]:2019": "localhost:2019",
	} {
		_, authority, err := adminURL(address)
		require.NoError(t, err, address)
		require.Equal(t, want, authority, address)
	}
}

// Measured on Caddy 2.10.2 (early smoke 3): the admin endpoint answers 403 to Host
// "localhost" and 200 to its listen authority "localhost:2019".
func TestAdminReadbackSendsTheAcceptedHost(t *testing.T) {
	x := fixture(t)
	x.execFn = func(args []string) hostio.ExecResult {
		if !slices.Contains(args, "Host: localhost:2019") {
			return httpResult(403, "")
		}
		return httpResult(200, "{}")
	}
	_, err := x.e.admin(t.Context(), x.i, "localhost:2019")
	require.NoError(t, err)
}

// A real timer ends the observation window with the child context's error; that is an
// applied-unverified outcome, not an uncertain exec (recovery-required).
func TestObservationWindowExpiryIsUnverified(t *testing.T) {
	x := fixture(t)
	x.execFn = func([]string) hostio.ExecResult { return httpResult(200, "{}") }
	x.e.wait = func(context.Context, time.Duration) error { return context.DeadlineExceeded }
	obs, err := x.e.Observe(t.Context(), observationIntent(x, baseSite(), false, "edit-route"), false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, obs.State)
	ctx, cancel := context.WithCancel(t.Context())
	x.e.wait = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	_, err = x.e.Observe(ctx, observationIntent(x, baseSite(), false, "edit-route"), false)
	require.ErrorIs(t, err, context.Canceled, "a cancelled operation context stays uncertain")
}
