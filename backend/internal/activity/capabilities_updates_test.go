// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Behavioural capability scope and revoked admission proofs for #229.

package activity

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/capability"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUpdateCapabilitiesScopedGrants(t *testing.T) {
	read := []string{"projects:read", "image-updates:read"}
	write := []string{"projects:deploy", "projects:update", "images:pull", "image-updates:check", "activities:read"}
	notify := []string{"projects:read", "image-updates:read", "image-updates:check"}
	apply := []string{"projects:read", "image-updates:read", "image-updates:check", "projects:deploy", "projects:update", "images:pull", "activities:read", "jobs:manage"}
	cases := map[string][]string{"checkUpdates": {"image-updates:check"}, "installStackUpdates": write, "restoreRestorePoint": write, "getUpdateBaseline": read, "listRestorePoints": read, "deleteRestorePoint": write, "getAutoUpdatePolicy": read, "setAutoUpdate": notify, "setAutoUpdateApply": apply, "listSchedule": {"jobs:manage"}, "saveUpdateSchedule": {"jobs:manage"}, "listSystemComponents": {"image-updates:read"}}
	for op, grants := range cases {
		t.Run(op, func(t *testing.T) {
			ref := capability.ResourceRef{Kind: "stack", ID: "example~stack"}
			if op == "listSchedule" || op == "saveUpdateSchedule" || op == "listSystemComponents" {
				ref = capability.ResourceRef{Kind: "host", ID: "example"}
			}
			allowed := authz.NewPermissionSet()
			for _, grant := range grants {
				allowed.AddEnv("example", grant)
			}
			require.True(t, networkDescriptor(t, allowed, ref, op).Available)
			for _, missing := range grants {
				ps := authz.NewPermissionSet()
				for _, grant := range grants {
					if grant != missing {
						ps.AddEnv("example", grant)
					} else {
						ps.AddEnv("foreign", grant)
					}
				}
				require.False(t, networkDescriptor(t, ps, ref, op).Available, missing)
			}
			for _, bad := range []capability.ResourceRef{{Kind: "hub", ID: "0"}, {Kind: "container", ID: "example~stack"}, {Kind: ref.Kind, ID: ""}, {Kind: ref.Kind, ID: "~stack"}, {Kind: ref.Kind, ID: "example~"}, {Kind: ref.Kind, ID: "example~stack~extra"}, {Kind: ref.Kind, ID: "foreign"}, {Kind: ref.Kind, ID: "foreign~stack"}} {
				require.False(t, networkDescriptor(t, allowed, bad, op).Available, "%+v", bad)
			}
		})
	}
	for _, op := range []string{"listHostRestorePoints", "deleteHostRestorePoint"} {
		ref := capability.ResourceRef{Kind: "host", ID: "example"}
		all := authz.NewPermissionSet()
		all.AddGlobal("image-updates:read")
		require.False(t, networkDescriptor(t, all, ref, op).Available)
		require.True(t, networkDescriptor(t, authz.SudoPermissionSet(), ref, op).Available)
		require.False(t, networkDescriptor(t, authz.SudoPermissionSet(), capability.ResourceRef{Kind: "hub", ID: "0"}, op).Available)
	}
}

func TestUpdateCapabilityReadThenRevokedCall(t *testing.T) {
	f := b1Fixture(t)
	c := imageCommand(f.command(), RuntimeRestoreProject)
	h, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) {
		t.Error("store accessed after revocation")
		return c.Store, nil
	}, PrepareTarget: func(context.Context, string, RuntimeTarget) (RuntimeResolvedTarget, error) {
		t.Error("target accessed after revocation")
		return RuntimeResolvedTarget{}, nil
	}})
	require.NoError(t, err)
	h.SetReady(true)
	for _, missing := range []string{"projects:deploy", "projects:update", "images:pull", "image-updates:check", "activities:read"} {
		before := imageGrantContext(c.Actor, "")
		ps, _ := middleware.PermissionsFromContext(before)
		require.True(t, networkDescriptor(t, ps, capability.ResourceRef{Kind: "stack", ID: "0~example-stack"}, "restoreRestorePoint").Available)
		_, _, err := h.admit(imageGrantContext(c.Actor, missing), "0", c.Key, c.Operation, c.Target, RuntimeOptions{Lifecycle: &dto.LifecycleIntent{Request: c.Options.Lifecycle.Request}})
		var denied huma.StatusError
		require.ErrorAs(t, err, &denied)
		require.Equal(t, 403, denied.GetStatus())
	}
	var receipts int64
	require.NoError(t, f.store.svc.db.Table("runtime_receipts").Count(&receipts).Error)
	require.Zero(t, receipts)
}

func TestPointOverlayRegistrationChange(t *testing.T) {
	f := b1Fixture(t)
	c := imageCommand(f.command(), RuntimeRestoreProject)
	c.Store.RegistrationID = "old-registration"
	accepted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	holds, err := f.store.svc.ReadPointHolds(t.Context(), "0", []string{c.Options.Lifecycle.Request.PointID})
	require.NoError(t, err)
	require.Equal(t, dto.HoldActive, holds[c.Options.Lifecycle.Request.PointID])
	require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", accepted.ActivityID).Updates(map[string]any{"executor_state": "unknown", "unknown_reason": "synthetic-transport-loss"}).Error)
	holds, err = f.store.svc.ReadPointHolds(context.Background(), "0", []string{c.Options.Lifecycle.Request.PointID})
	require.NoError(t, err)
	require.Equal(t, dto.HoldUnknown, holds[c.Options.Lifecycle.Request.PointID])
	holds, err = f.store.svc.ReadPointHolds(t.Context(), "foreign", []string{c.Options.Lifecycle.Request.PointID})
	require.NoError(t, err)
	require.Empty(t, holds)
}

func TestUpdateCapabilitiesTransportParity(t *testing.T) {
	stack := []string{"checkUpdates", "installStackUpdates", "restoreRestorePoint", "getUpdateBaseline", "listRestorePoints", "deleteRestorePoint", "getAutoUpdatePolicy", "setAutoUpdate", "setAutoUpdateApply"}
	host := []string{"listHostRestorePoints", "deleteHostRestorePoint", "listSchedule", "saveUpdateSchedule", "listSystemComponents"}
	for _, env := range []string{"0", "direct-example", "edge-example"} {
		for _, op := range stack {
			require.True(t, networkDescriptor(t, authz.SudoPermissionSet(), capability.ResourceRef{Kind: "stack", ID: env + "~stack"}, op).Available, env+"/"+op)
		}
		for _, op := range host {
			require.True(t, networkDescriptor(t, authz.SudoPermissionSet(), capability.ResourceRef{Kind: "host", ID: env}, op).Available, env+"/"+op)
		}
	}
	for _, route := range []struct{ method, path, command string }{
		{"GET", "/updater/projects/stack/baseline", "updater.project.baseline"},
		{"GET", "/updater/project/stack/restore-points", "updater.restore.list"},
		{"DELETE", "/updater/project/stack/restore-points/point", "updater.restore.delete"},
		{"GET", "/updater/restore-points", "updater.restore.host-list"},
		{"DELETE", "/updater/restore-points/point", "updater.restore.host-delete"},
	} {
		command, ok := edge.ResolveEdgeCommandName(route.method, "/api/environments/example"+route.path, false).Get()
		require.True(t, ok, route.path)
		require.Equal(t, route.command, command)
	}
}
