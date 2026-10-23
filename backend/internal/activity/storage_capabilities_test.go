// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4037e18b9c926793e2888952b4ec9c0805ecc62e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original committed volume and workspace descriptor rights matrix for #183.

package activity

import (
	"context"
	"fmt"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	"github.com/stretchr/testify/require"
)

func storageDescriptor(t *testing.T, permissions *authz.PermissionSet, ref capability.ResourceRef, operation string) bool {
	t.Helper()
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, permissions)
	result, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: ref.Kind, ID: ref.ID})
	require.NoError(t, err)
	for _, descriptor := range result.Body.Data {
		if descriptor.OperationID != operation {
			continue
		}
		require.Equal(t, ref, descriptor.ResourceRef)
		require.Equal(t, !descriptor.Available, descriptor.DisabledReasonCode != "")
		return descriptor.Available
	}
	t.Fatalf("missing descriptor %s", operation)
	return false
}

func TestVolumeCapabilitiesRequireScopedRights(t *testing.T) {
	grants := map[string]string{
		"listVolumes": authz.PermVolumesList, "getVolume": authz.PermVolumesRead,
		"createVolume": authz.PermVolumesCreate, "deleteVolume": authz.PermVolumesDelete,
		"renameVolume": authz.PermVolumesRename, "planVolumePrune": authz.PermVolumesPrune,
		"pruneVolumes": authz.PermVolumesPrune,
	}
	for operation, grant := range grants {
		for _, allowed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/allowed=%t", operation, allowed), func(t *testing.T) {
				permissions := authz.NewPermissionSet()
				if allowed {
					permissions.PerEnv["example"] = map[string]struct{}{grant: {}}
				}
				kind, id, foreign := "host", "example", "other"
				if operation == "deleteVolume" || operation == "renameVolume" {
					kind, id, foreign = "volume", "example~data", "other~data"
				}
				for _, tc := range []struct {
					kind, id string
					want     bool
				}{
					{kind, id, allowed}, {"hub", "0", allowed}, {kind, foreign, false},
					{kind, "", false}, {kind, "~data", false}, {kind, "example~", false},
					{kind, "example~data~extra", false}, {"stack", "example~data", false},
				} {
					require.Equal(t, tc.want, storageDescriptor(t, permissions, capability.ResourceRef{Kind: tc.kind, ID: tc.id}, operation), "%s %s", tc.kind, tc.id)
				}
				global := &authz.PermissionSet{Global: map[string]struct{}{grant: {}}}
				require.True(t, storageDescriptor(t, global, capability.ResourceRef{Kind: kind, ID: id}, operation))
				require.True(t, storageDescriptor(t, global, capability.ResourceRef{Kind: "hub", ID: "0"}, operation))
			})
		}
	}
}

func TestVolumeWorkspaceDescriptorRightsMasks(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		for operation, required := range map[string]int{"listFiles": 1, "readFile": 1, "downloadFile": 1, "createEntry": 3, "writeFile": 3, "uploadFile": 3, "renameEntry": 7, "deleteEntry": 5} {
			t.Run(fmt.Sprintf("%s/mask=%d", operation, mask), func(t *testing.T) {
				grants := map[string]struct{}{}
				for bit, grant := range []string{authz.PermVolumesRead, authz.PermVolumesUpload, authz.PermVolumesDelete} {
					if mask&(1<<bit) != 0 {
						grants[grant] = struct{}{}
					}
				}
				permissions := &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{"example": grants}}
				require.Equal(t, mask&required == required, storageDescriptor(t, permissions, capability.ResourceRef{Kind: "volume", ID: "example~data"}, operation))
				for _, ref := range []capability.ResourceRef{
					{Kind: "volume", ID: "other~data"}, {Kind: "volume", ID: "example~bind:/data"},
					{Kind: "volume", ID: "example~data~extra"}, {Kind: "volume", ID: "example"},
					{Kind: "volume", ID: "~data"}, {Kind: "volume", ID: "example~"}, {Kind: "volume", ID: ""},
					{Kind: "hub", ID: "0"}, {Kind: "host", ID: "example"}, {Kind: "stack", ID: "example~data"},
				} {
					require.False(t, storageDescriptor(t, permissions, ref, operation), "%+v", ref)
				}
			})
		}
	}
}

func TestProjectWorkspaceDescriptorRightsMasks(t *testing.T) {
	for mask := 0; mask < 4; mask++ {
		grants := map[string]struct{}{}
		for bit, grant := range []string{authz.PermProjectsRead, authz.PermProjectsUpdate} {
			if mask&(1<<bit) != 0 {
				grants[grant] = struct{}{}
			}
		}
		permissions := &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{"example": grants}}
		for operation, required := range map[string]int{"getProjectWorkspace": 1, "readProjectFile": 1, "downloadProjectFile": 1, "updateProjectWorkspace": 2} {
			t.Run(fmt.Sprintf("%s/mask=%d", operation, mask), func(t *testing.T) {
				require.Equal(t, mask&required == required, storageDescriptor(t, permissions, capability.ResourceRef{Kind: "stack", ID: "example~project"}, operation))
				for _, ref := range []capability.ResourceRef{
					{Kind: "stack", ID: "other~project"}, {Kind: "stack", ID: "example~project~extra"},
					{Kind: "stack", ID: "example"}, {Kind: "stack", ID: "~project"}, {Kind: "stack", ID: "example~"},
					{Kind: "stack", ID: ""}, {Kind: "hub", ID: "0"}, {Kind: "host", ID: "example"}, {Kind: "volume", ID: "example~project"},
				} {
					require.False(t, storageDescriptor(t, permissions, ref, operation), "%+v", ref)
				}
			})
		}
	}
}
