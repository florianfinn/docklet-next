// Source: docklet-next/docs/design/network-management.md
// Revision: 81b69b33c80a454465f54a1f7289c1bc5b5a3381
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	"github.com/stretchr/testify/require"
	"testing"
)

func networkDescriptor(t *testing.T, permissions *authz.PermissionSet, ref capability.ResourceRef, operation string) capability.Descriptor {
	t.Helper()
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, permissions)
	response, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: ref.Kind, ID: ref.ID})
	require.NoError(t, err)
	for _, d := range response.Body.Data {
		if d.OperationID == operation {
			require.Equal(t, !d.Available, d.DisabledReasonCode != "")
			return d
		}
	}
	t.Fatalf("Missing operation %s", operation)
	return capability.Descriptor{}
}
func TestNetworkCapabilitiesRequireScopedRights(t *testing.T) {
	for operation, grant := range map[string]string{"createNetwork": authz.PermNetworksCreate, "removeNetwork": authz.PermNetworksDelete, "connectNetwork": authz.PermNetworksConnect, "disconnectNetwork": authz.PermNetworksDisconnect} {
		t.Run(operation, func(t *testing.T) {
			kind, id, foreign := "network", "example~net1", "other~net1"
			if operation == "createNetwork" {
				kind, id, foreign = "host", "example", "other"
			}
			permissions := authz.NewPermissionSet()
			permissions.AddEnv("example", grant)
			for _, tc := range []struct {
				ref  capability.ResourceRef
				want bool
			}{
				{capability.ResourceRef{Kind: kind, ID: id}, true},
				{capability.ResourceRef{Kind: "hub", ID: "0"}, true},
				{capability.ResourceRef{Kind: kind, ID: foreign}, false},
				{capability.ResourceRef{Kind: kind, ID: ""}, false},
				{capability.ResourceRef{Kind: kind, ID: "~net1"}, false},
				{capability.ResourceRef{Kind: kind, ID: "example~"}, false},
				{capability.ResourceRef{Kind: kind, ID: "example~net1~extra"}, false},
				{capability.ResourceRef{Kind: "container", ID: "example~net1"}, false},
			} {
				require.Equal(t, tc.want, networkDescriptor(t, permissions, tc.ref, operation).Available, "%+v", tc.ref)
			}
			ref := capability.ResourceRef{Kind: kind, ID: id}
			require.False(t, networkDescriptor(t, authz.NewPermissionSet(), ref, operation).Available)
			global := &authz.PermissionSet{Global: map[string]struct{}{grant: {}}}
			require.True(t, networkDescriptor(t, global, ref, operation).Available)
		})
	}
}
