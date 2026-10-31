// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D4 shell descriptor permission and availability matrix.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestShellCapabilityD4PermissionAndState(t *testing.T) {
	for _, tc := range []struct {
		name, grant, reason string
		available           bool
	}{
		{"running", authz.PermContainersExec, "", true},
		{"standby-running", authz.PermContainersExec, "", true},
		{"stopped", authz.PermContainersExec, "container.shell-not-running", false},
		{"offline", authz.PermContainersExec, "agent.unavailable", false},
		{"credentials", authz.PermContainersExec, "agent.invalid-token", false},
		{"old-agent", authz.PermContainersExec, "agent.update-required", false},
		{"read-is-not-exec", authz.PermContainersRead, "", false},
		{"start-is-not-exec", authz.PermContainersStart, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{"example": {tc.grant: {}}}}
			ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, ps)
			called := false
			ctx = containershell.WithStateReader(ctx, func(_ context.Context, env, raw string) string {
				called = true
				require.Equal(t, "example", env)
				require.Equal(t, "container", raw)
				return tc.reason
			})
			out, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: "container", ID: "example~container"})
			require.NoError(t, err)
			var descriptor *capability.Descriptor
			for i := range out.Body.Data {
				if out.Body.Data[i].OperationID == "openContainerShell" {
					descriptor = &out.Body.Data[i]
				}
			}
			require.NotNil(t, descriptor)
			require.Equal(t, tc.available, descriptor.Available)
			if tc.grant != authz.PermContainersExec {
				require.False(t, called)
				require.Equal(t, "capability.unavailable", descriptor.DisabledReasonCode)
			} else {
				require.Equal(t, tc.reason, descriptor.DisabledReasonCode)
			}
		})
	}
	for _, ref := range []capability.ResourceRef{{Kind: "hub", ID: "0"}, {Kind: "host", ID: "example"}, {Kind: "container", ID: "example"}, {Kind: "container", ID: "example~a~b"}} {
		ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		ctx = containershell.WithStateReader(ctx, func(context.Context, string, string) string { t.Error("invalid ref read state"); return "" })
		out, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: ref.Kind, ID: ref.ID})
		require.NoError(t, err)
		for _, descriptor := range out.Body.Data {
			if descriptor.OperationID == "openContainerShell" {
				require.False(t, descriptor.Available)
			}
		}
	}
}
