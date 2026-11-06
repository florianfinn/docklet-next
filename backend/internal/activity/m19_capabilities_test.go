// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original per-item redeploy capability handler regression guards.

package activity

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/capability"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestM19RedeployCapabilityHandlerRestrictedVersusFull(t *testing.T) {
	for _, level := range []string{"restricted", "full"} {
		t.Run(level, func(t *testing.T) {
			_, api := humatest.New(t)
			api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
				ps := access.Synthesize("reader", []userDTO.AppAssignment{{EnvironmentID: "host", ProjectID: "app", Level: level}})
				next(huma.WithContext(c, context.WithValue(c.Context(), middleware.ContextKeyUserPermissions, ps)))
			})
			RegisterCapabilities(api)
			response := api.Get("/capabilities?kind=stack&id=host~app")
			require.Equal(t, 200, response.Code, response.Body.String())
			var result base.ApiResponse[[]capability.Descriptor]
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			found := map[string]capability.Descriptor{}
			for _, d := range result.Data {
				found[d.OperationID] = d
			}
			require.True(t, found["runStackCommand"].Available)
			require.Equal(t, level == "full", found["redeployStack"].Available)
			if level == "restricted" {
				require.Equal(t, "access.full-required", found["redeployStack"].DisabledReasonCode)
			}
		})
	}
}
