// Source: docklet-next/docs/design/images-updates.md
// Revision: 7df52f8f3b912be751e2283ec8d990933f5dc4b8
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original image/registry UI foundation follow-up for issue #229.

package activity

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	"github.com/stretchr/testify/require"
)

func TestImageCapabilitiesRequireExactScopeAndGrant(t *testing.T) {
	for operation, grant := range map[string]string{"listImages": authz.PermImagesList, "pullImage": authz.PermImagesPull, "removeImage": authz.PermImagesDelete, "pruneImages": authz.PermImagesPrune} {
		t.Run(operation, func(t *testing.T) {
			kind, id := "host", "example"
			if operation == "removeImage" {
				kind, id = "image", "example~sha256:abc"
			}
			allowed := authz.NewPermissionSet()
			allowed.AddEnv("example", grant)
			for _, ref := range []capability.ResourceRef{{Kind: kind, ID: id}, {Kind: "hub", ID: "0"}} {
				require.True(t, networkDescriptor(t, allowed, ref, operation).Available)
				require.True(t, networkDescriptor(t, authz.SudoPermissionSet(), ref, operation).Available)
				require.False(t, networkDescriptor(t, authz.NewPermissionSet(), ref, operation).Available)
			}
			for _, ref := range []capability.ResourceRef{{Kind: kind, ID: "foreign"}, {Kind: kind, ID: "foreign~image"}, {Kind: kind, ID: ""}, {Kind: kind, ID: "~image"}, {Kind: kind, ID: "example~"}, {Kind: kind, ID: "example~image~extra"}, {Kind: "container", ID: "example~image"}, {Kind: "hub", ID: "foreign"}} {
				require.False(t, networkDescriptor(t, allowed, ref, operation).Available, "%+v", ref)
			}
			wrongKind := "image"
			if operation == "removeImage" {
				wrongKind = "host"
			}
			require.False(t, networkDescriptor(t, authz.SudoPermissionSet(), capability.ResourceRef{Kind: wrongKind, ID: id}, operation).Available)
		})
	}
}

func TestRegistryCapabilitiesAreHubOwnedAndGloballyGranted(t *testing.T) {
	for operation, grant := range map[string]string{"listContainerRegistries": authz.PermRegistriesList, "getContainerRegistry": authz.PermRegistriesRead, "createContainerRegistry": authz.PermRegistriesCreate, "updateContainerRegistry": authz.PermRegistriesUpdate, "clearContainerRegistryCredential": authz.PermRegistriesUpdate, "testContainerRegistry": authz.PermRegistriesTest, "deleteContainerRegistry": authz.PermRegistriesDelete} {
		t.Run(operation, func(t *testing.T) {
			global := authz.NewPermissionSet()
			global.AddGlobal(grant)
			local := authz.NewPermissionSet()
			local.AddEnv("example", grant)
			for _, ref := range []capability.ResourceRef{{Kind: "hub", ID: "0"}, {Kind: "registry", ID: "registry-1"}} {
				want := ref.Kind == "hub" || (operation != "listContainerRegistries" && operation != "createContainerRegistry")
				require.Equal(t, want, networkDescriptor(t, global, ref, operation).Available)
				require.False(t, networkDescriptor(t, local, ref, operation).Available)
				require.False(t, networkDescriptor(t, authz.NewPermissionSet(), ref, operation).Available)
			}
			for _, ref := range []capability.ResourceRef{{Kind: "host", ID: "example"}, {Kind: "registry", ID: "example~registry-1"}, {Kind: "registry", ID: ""}, {Kind: "hub", ID: "foreign"}} {
				require.False(t, networkDescriptor(t, authz.SudoPermissionSet(), ref, operation).Available)
			}
		})
	}
}
