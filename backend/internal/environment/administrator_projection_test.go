// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package environment

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAdministratorKeyEnvironmentPayloadPreserved(t *testing.T) {
	for _, transport := range []string{"REST", "SSE"} {
		t.Run(transport, func(t *testing.T) {
			db := setupEnvironmentServiceTestDB(t)
			svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)
			require.NoError(t, db.Create(&Environment{ID: "assigned", Name: "Example", ApiUrl: "https://agent.example.com", Enabled: true, Status: "online", AccessToken: new("example")}).Error)
			h := NewHandler(svc, nil, nil, nil, nil, nil)
			// Scoped administrator keys carry grants without a simple-user identity.
			ps := authz.NewPermissionSet()
			ps.AddEnv("assigned", authz.PermContainersRead)
			var rows []dto.Environment
			if transport == "REST" {
				ps.AddGlobal(authz.PermEnvironmentsList)
				ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, ps)
				out, err := h.ListEnvironments(ctx, &ListEnvironmentsInput{Limit: 20})
				require.NoError(t, err)
				rows = out.Body.Data
			} else {
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan struct{})
				events := make(chan dto.StreamEvent, 1)
				go func() { defer close(done); h.RunStreamProducer(ctx, ps, events) }()
				defer func() { cancel(); <-done }()
				select {
				case event := <-events:
					rows = event.Environments
				case <-time.After(3 * time.Second):
					t.Fatal("environment producer did not publish")
				}
			}
			require.Len(t, rows, 1)
			require.False(t, rows[0].DisplayOnly)
			require.Equal(t, "https://agent.example.com", rows[0].ApiUrl)
			require.True(t, rows[0].Enabled)
		})
	}
}
