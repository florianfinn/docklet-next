// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D7 descriptor and D6 refusal parity matrix.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/stretchr/testify/require"
)

func TestD7HostDescriptors(t *testing.T) {
	for _, op := range []string{"updateHost", "connectHost", "reconnectHost", "restartAgent"} {
		t.Run(op, func(t *testing.T) {
			for _, tc := range []struct {
				name, reason         string
				deny, malformed, hub bool
			}{
				{name: "allowed"}, {name: "denied", deny: true}, {name: "malformed", malformed: true},
				{name: "hub", hub: true}, {name: "host.restarting", reason: "host.restarting"},
				{name: "agent.update-required", reason: "agent.update-required"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ps := authz.SudoPermissionSet()
					if op == "connectHost" || op == "reconnectHost" {
						ps = &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{"example": {authz.PermEnvironmentsRead: {}}, "0": {authz.PermEnvironmentsRead: {}}}}
					}
					if tc.deny {
						ps = &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{"example": {authz.PermEnvironmentsUpdate: {}}}}
					}
					ref := capability.ResourceRef{Kind: "host", ID: "example"}
					if tc.malformed {
						ref.ID += "~target"
					}
					if tc.hub {
						ref.ID = "0"
					}
					reads := 0
					read := func(context.Context, string) string {
						reads++
						if tc.reason == "host.restarting" {
							return ""
						}
						return tc.reason
					}
					ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, ps)
					ctx = WithOperationCapabilityReaders(ctx, OperationCapabilityReaders{
						HostReason: read, HostOperationReason: read,
						RestartReason: func(context.Context, string) string {
							reads++
							if tc.reason == "host.restarting" {
								return tc.reason
							}
							return ""
						},
					})
					want := tc.reason
					if tc.deny || tc.malformed || (tc.hub && op == "restartAgent") {
						want = "capability.unavailable"
					}
					out, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: ref.Kind, ID: ref.ID})
					require.NoError(t, err)
					found := false
					for _, d := range out.Body.Data {
						if d.OperationID == op {
							found = true
							require.Equal(t, want, d.DisabledReasonCode)
							require.Equal(t, want == "", d.Available)
						}
					}
					require.True(t, found)
					reads = 0
					require.Equal(t, want, operationCapability(ctx, op, ref, ps))
					if want == "capability.unavailable" {
						require.Zero(t, reads)
					} else {
						require.Positive(t, reads)
					}
				})
			}
		})
	}
}

func TestD6RestartAndSettingsParity(t *testing.T) {
	for _, op := range []string{"recreateContainer", "setContainerOrigin", "restartService"} {
		t.Run(op, func(t *testing.T) {
			ref := capability.ResourceRef{Kind: "container", ID: "example~target"}
			if op == "restartService" {
				ref.Kind = "stack"
			}
			ctx := WithOperationCapabilityReaders(t.Context(), OperationCapabilityReaders{
				RestartReason: func(context.Context, string) string { return "host.restarting" },
				HostReason:    func(context.Context, string) string { return "" },
				Origin: func(context.Context, string, string) (dto.OriginFields, error) {
					t.Fatal("origin read during restart")
					return dto.OriginFields{}, nil
				},
				GuardOrigin: func(context.Context, string, string) error { t.Fatal("guard read during restart"); return nil },
			})
			require.Equal(t, "host.restarting", operationCapability(ctx, op, ref, authz.SudoPermissionSet()))
		})
	}
	ctx := WithOperationCapabilityReaders(t.Context(), OperationCapabilityReaders{SettingsWriteReason: func(context.Context) string { return "settings.ui-configuration-disabled" }})
	ref := capability.ResourceRef{Kind: "hub", ID: "0"}
	require.Equal(t, "settings.ui-configuration-disabled", operationCapability(ctx, "saveSettings", ref, authz.SudoPermissionSet()))
	require.Empty(t, operationCapability(ctx, "getSettings", ref, authz.SudoPermissionSet()))
}

func TestD7RestartPredicateFresh(t *testing.T) {
	f := runtimeFixture(t)
	db := f.svc.db.DB
	ctx := t.Context()
	require.Empty(t, RestartCapabilityReason(ctx, db, "example"))
	now := time.Now()
	require.NoError(t, db.Create(&environment.AgentRestartRequest{RunID: "example-restart", EnvironmentID: "example", RegistrationID: "example", State: "reserved", CreatedAt: now, UpdatedAt: now, Deadline: now.Add(time.Minute)}).Error)
	require.Equal(t, "host.restarting", RestartCapabilityReason(ctx, db, "example"))
	require.Empty(t, RestartCapabilityReason(ctx, db, "other"))
	require.NoError(t, db.Model(&environment.AgentRestartRequest{}).Where("run_id=?", "example-restart").Updates(map[string]any{"state": "connected", "settled_at": now}).Error)
	require.Empty(t, RestartCapabilityReason(ctx, db, "example"))
	require.Equal(t, "capability.unavailable", RestartCapabilityReason(ctx, nil, "example"))
}
