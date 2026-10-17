// Source: docklet-next/issues/81
// Revision: e0a44b221176de20524bf7829741b1246461c927
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original nil-tunnel binding regression tests.

package edge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionTunnelNeverPinsNil(t *testing.T) {
	for _, name := range []string{"helper", "typed-nil"} {
		t.Run(name, func(t *testing.T) {
			ctx := WithSessionCredential(t.Context(), "example", "test-only", nil)
			if name == "helper" {
				ctx = WithSessionTunnel(ctx, nil)
				require.True(t, ctx.Value(sessionTunnelContext{}) == nil, "absence is not a concrete verdict")
			} else {
				ctx = context.WithValue(ctx, sessionTunnelContext{}, (*AgentTunnel)(nil))
			}
			tunnel := &AgentTunnel{APIKeyID: "example", TokenHash: AgentTokenHash("test-only")}
			require.NoError(t, RequireSessionKey(ctx, tunnel))
		})
	}
}
