// Source: docklet-next/issues/292
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original route test for the shared agent access envelope.

package settings

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
)

func TestRemoteSettingsRouteUsesAgentAccessEnvelope(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("synthetic-private.example.com: %w", remenv.ErrAgentUpdateRequired), http.StatusConflict, "agent.update-required"},
		{&remenv.TransportError{Err: remenv.ErrEnvironmentUnavailable}, http.StatusBadGateway, "agent.unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			_, api := humatest.New(t)
			proxy := func(context.Context, string, string, string, []byte, any) error { return tc.err }
			RegisterSettings(api, nil, nil, proxy, nil)
			resp := api.Get("/environments/env-remote/settings/public")
			require.Equal(t, tc.status, resp.Code)
			require.JSONEq(t, `{"success":false,"code":"`+tc.code+`"}`, resp.Body.String())
		})
	}
}
