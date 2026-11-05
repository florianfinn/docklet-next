// Source: docklet-next/issues/342
// Revision: d3a0e226162db637196e463dbca0ce89726275a1
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: A policy-owned container update is admitted under its policy child ID.

package activity

import (
	"context"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestPolicyContainerUpdateUsesChildID(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	h, e := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) { return c.Store, nil }, PrepareTarget: func(_ context.Context, _ string, target RuntimeTarget) (RuntimeResolvedTarget, error) {
		target.ID, target.Name = strings.Repeat("a", 64), new("app")
		return RuntimeResolvedTarget{Target: target}, nil
	}, GuardContainer: func(context.Context, string, string) error { return nil }})
	require.NoError(t, e)
	h.SetReady(true)
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
	ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &c.Actor)
	store := policy.Store{DB: f.store.svc.db.DB}
	row, e := store.Policy(t.Context(), c.Store.EnvironmentID, c.Store.RegistrationID, "app", "container")
	require.NoError(t, e)
	row.Mode, row.ActorID, row.TargetIdentity = "scheduled", c.Actor.ID, "app"
	row.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
	require.NoError(t, f.store.svc.db.Exec("INSERT OR IGNORE INTO environments(id,name,api_url,enabled) VALUES(?,?,?,1)", c.Store.EnvironmentID, "Example", "https://agent.example.com").Error)
	require.NoError(t, store.SavePolicy(t.Context(), row, 0))
	row.Revision = 1
	prepared, _, e := store.PrepareChild(t.Context(), row, "window")
	require.NoError(t, e)
	child := prepared.ID
	binding := dto.PolicyBinding{PolicyID: row.ID, Revision: 1, JobRunID: "window", ChildID: child}
	accepted, _, e := h.admit(ctx, "0", child, RuntimeUpdateContainer, RuntimeTarget{Kind: "container", ID: "app"}, RuntimeOptions{ContainerImage: &dto.ContainerImageRequest{Policy: &binding}})
	require.NoError(t, e)
	require.Equal(t, child, accepted.ActivityID, "the policy waits on its child ID, so the receipt must carry it")
}
