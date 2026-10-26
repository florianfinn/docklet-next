// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Review correction with real producer and behavioral guard proofs.

package updater

import (
	"context"
	"encoding/json"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
)

type baselineProxyStub struct {
	t       *testing.T
	success bool
	calls   int
}

func (p *baselineProxyStub) ProxyJSONRequest(_ context.Context, env, method, path string, body []byte, out any) error {
	p.calls++
	require.Equal(p.t, "edge-example", env)
	require.Equal(p.t, http.MethodGet, method)
	require.Equal(p.t, "/api/environments/0/updater/projects/example-stack/baseline", path)
	require.Nil(p.t, body)
	b, e := json.Marshal(map[string]any{"success": p.success, "data": dto.BaselineStatus{TargetIdentity: strings.Repeat("a", 64), SavedRevision: strings.Repeat("b", 64), ObservedRevision: strings.Repeat("c", 64)}})
	require.NoError(p.t, e)
	return json.Unmarshal(b, out)
}
func TestEdgeBaselineConfiguredEnvelope(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(map[bool]string{true: "success", false: "failed"}[success], func(t *testing.T) {
			f := newPolicyFixture(t)
			proxy := &baselineProxyStub{t: t, success: success}
			f.p.ports.baseline = f.s.imagePolicyBaseline(proxy)
			_, err := f.s.GetImagePolicy(t.Context(), "edge-example", "example-stack")
			if success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, 1, proxy.calls)
		})
	}
}
