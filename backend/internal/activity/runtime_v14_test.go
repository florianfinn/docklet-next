// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original service-set ownership, digest and frozen instance admission proofs.

package activity

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestV14LifecycleServiceSetOwnedAndDigested(t *testing.T) {
	f := b1Fixture(t)
	c := imageCommand(f.command(), RuntimeUpdateProject)
	services := []string{"worker", "app", "app"}
	c.Options.Lifecycle.Request.Services = services
	frozen, e := runtimeCanonicalizeInternal(c)
	require.NoError(t, e)
	services[0] = "outside-change"
	require.Equal(t, []string{"app", "worker"}, frozen.command.Options.Lifecycle.Request.Services)
	same := imageCommand(f.command(), RuntimeUpdateProject)
	same.Options.Lifecycle.Request.Services = []string{"app", "worker"}
	canonical, e := runtimeCanonicalizeInternal(same)
	require.NoError(t, e)
	require.Equal(t, frozen.digest, canonical.digest)
	same.Options.Lifecycle.Request.Services = []string{"app"}
	changed, e := runtimeCanonicalizeInternal(same)
	require.NoError(t, e)
	require.NotEqual(t, frozen.digest, changed.digest)
	admitted, e := f.service.Admit(t.Context(), frozen.command)
	require.NoError(t, e)
	var row RuntimeImageIntent
	require.NoError(t, f.store.svc.db.First(&row, "run_id = ?", admitted.ActivityID).Error)
	var intent dto.LifecycleIntent
	require.NoError(t, json.Unmarshal([]byte(row.Payload), &intent))
	require.Equal(t, []string{"app", "worker"}, intent.Request.Services)
	retry := frozen.command
	retry.Options.Lifecycle.Request.Services = []string{"worker", "app", "worker"}
	retained, found, e := f.service.RetainedCommand(t.Context(), retry)
	require.NoError(t, e)
	require.True(t, found)
	require.Equal(t, []string{"app", "worker"}, retained.Options.Lifecycle.Request.Services)
}

func TestV14ContainerAdmissionFreezesResolvedID(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	first := strings.Repeat("a", 64)
	current := first
	h, e := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) { return c.Store, nil }, PrepareTarget: func(_ context.Context, _ string, target RuntimeTarget) (RuntimeResolvedTarget, error) {
		target.ID = current
		return RuntimeResolvedTarget{Target: target}, nil
	}, GuardContainer: func(context.Context, string, string) error { return nil }})
	require.NoError(t, e)
	h.SetReady(true)
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
	ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &c.Actor)
	accepted, _, e := h.admit(ctx, "0", c.Key, RuntimeUpdateContainer, RuntimeTarget{Kind: "container", ID: "app"}, RuntimeOptions{})
	require.NoError(t, e)
	current = strings.Repeat("b", 64)
	var receipt RuntimeReceipt
	require.NoError(t, f.store.svc.db.First(&receipt, "run_id = ?", accepted.ActivityID).Error)
	require.NotNil(t, receipt.TargetID)
	require.Equal(t, first, *receipt.TargetID)
}
