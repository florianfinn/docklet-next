// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package environment

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/environment"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestUserEnvironmentProjectionKeepsSharedProducerComplete(t *testing.T) {
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)
	for _, id := range []string{"assigned", "foreign"} {
		require.NoError(t, db.Create(&Environment{ID: id, Name: id, ApiUrl: "https://agent.example.com", Enabled: true, Status: "online", AccessToken: new("example")}).Error)
	}
	h := NewHandler(svc, nil, nil, nil, nil, nil)
	user := access.Synthesize("viewer", []userDTO.AppAssignment{{EnvironmentID: "assigned", ProjectID: "uuid", Level: "restricted"}})
	admin := authz.SudoPermissionSet()
	life, cancel := context.WithCancel(t.Context())
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	receive := func(ps *authz.PermissionSet) dto.StreamEvent {
		events := make(chan dto.StreamEvent, 4)
		workers.Go(func() { h.RunStreamProducer(life, ps, events) })
		select {
		case e := <-events:
			return e
		case <-time.After(3 * time.Second):
			t.Fatal("environment producer did not publish")
			return dto.StreamEvent{}
		}
	}
	first := receive(admin)
	require.Len(t, first.Environments, 2)
	projected := receive(user)
	require.Len(t, projected.Environments, 1)
	raw, err := json.Marshal(projected.Environments[0])
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	require.ElementsMatch(t, []string{"id", "name", "status", "color"}, keysM15(fields))
	after := receive(admin)
	require.Len(t, after.Environments, 2)
	for _, env := range after.Environments {
		require.Equal(t, "https://agent.example.com", env.ApiUrl)
		require.False(t, env.DisplayOnly)
	}
	var readers sync.WaitGroup
	for range 10 {
		for _, ps := range []*authz.PermissionSet{admin, user} {
			readers.Go(func() {
				ctx := context.WithValue(life, middleware.ContextKeyUserPermissions, ps)
				out, err := h.ListEnvironments(ctx, &ListEnvironmentsInput{Limit: 20})
				require.NoError(t, err)
				if ps.IsGlobalAdmin() {
					require.Len(t, out.Body.Data, 2)
					for _, env := range out.Body.Data {
						require.Equal(t, "https://agent.example.com", env.ApiUrl)
					}
				} else {
					require.Len(t, out.Body.Data, 1)
					require.True(t, out.Body.Data[0].DisplayOnly)
				}
			})
		}
	}
	readers.Wait()
}
func keysM15(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}
