// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original shared process identity contract proof through the health route.
package health

import (
	"encoding/json"
	"github.com/danielgtaylor/huma/v2/humatest"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHealthProcessInstanceIdentityIsStableAndShared(t *testing.T) {
	_, api := humatest.New(t)
	RegisterRoutes(api, nil)
	first := InstanceID()
	_, err := uuid.Parse(first)
	require.NoError(t, err)
	for range 2 {
		response := api.Get("/health")
		require.Equal(t, 200, response.Code)
		var body dto.HealthResponse
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		require.Equal(t, first, body.InstanceID)
		require.Equal(t, first, InstanceID())
	}
}
