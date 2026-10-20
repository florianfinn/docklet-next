// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original scoped log capability tests; no new grants.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRuntimeLogCapabilities(t *testing.T) {
	for _, family := range []struct{ kind, operation, read, logs string }{
		{"stack", "subscribeLogs", authz.PermProjectsRead, authz.PermProjectsLogs},
		{"container", "subscribeContainerLogs", authz.PermContainersRead, authz.PermContainersLogs},
	} {
		t.Run(family.kind, func(t *testing.T) {
			for _, grants := range [][]string{nil, {family.read}, {family.logs}, {family.read, family.logs}} {
				permissions := authz.NewPermissionSet()
				for _, grant := range grants {
					permissions.AddEnv("example-host", grant)
				}
				ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, permissions)
				for _, ref := range []struct {
					kind, id string
					allowed  bool
				}{{family.kind, "example-host~resource", len(grants) == 2}, {family.kind, "other-host~resource", false}, {family.kind, "resource", false}, {family.kind, "example-host~resource~extra", false}, {"hub", "0", false}, {"volume", "example-host~resource", false}} {
					result, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: ref.kind, ID: ref.id})
					require.NoError(t, err)
					found := false
					for _, d := range result.Body.Data {
						if d.OperationID == family.operation {
							found = true
							require.Equal(t, ref.allowed, d.Available)
							require.Equal(t, !ref.allowed, d.DisabledReasonCode != "")
						}
						switch d.OperationID {
						case "runStackCommand", "runContainerCommand", "restartService", "deployStack":
							require.False(t, d.Available)
						}
					}
					require.True(t, found)
				}
			}
		})
	}
}
