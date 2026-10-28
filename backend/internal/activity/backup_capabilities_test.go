// Source: docklet-next/docs/design/volume-backups.md
// Revision: 7139e2bc64162c47b9f027ecbcaf1b06604c97c9
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original backup descriptor rights matrix for #284.

package activity

import (
	"fmt"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	"github.com/stretchr/testify/require"
)

func TestBackupCapabilitiesRequireRouteRights(t *testing.T) {
	// Expected rights restate the registered routes, independent of the implementation tables.
	volume := map[string][]string{
		"setBackupPolicy": {authz.PermVolumesBackup}, "runBackup": {authz.PermVolumesBackup},
		"restoreBackup": {authz.PermVolumesBackup}, "restoreBackupFiles": {authz.PermVolumesBackup},
		"deleteBackup": {authz.PermVolumesBackup}, "browseBackup": {authz.PermVolumesRead},
		"downloadBackup": {authz.PermVolumesRead}, "uploadRestore": {authz.PermVolumesBackup, authz.PermVolumesUpload},
	}
	host := map[string][]string{
		"getBackupOverview": {authz.PermVolumesList}, "getBackupRepository": {authz.PermVolumesRead},
		"checkBackupRepository": {authz.PermVolumesBackup}, "getBackupTargetReferences": {authz.PermVolumesRead},
		"testBackupTarget": {authz.PermVolumesBackup}, "getBackupRecoveryKey": {authz.PermSystemBackupsRestore},
		"attachLocalBackupRecoveryKey": {authz.PermSystemBackupsRestore},
	}
	scoped := map[string][]string{}
	for operation, grants := range volume {
		scoped[operation] = grants
	}
	for operation, grants := range host {
		scoped[operation] = grants
	}
	for operation, grants := range scoped {
		_, onVolume := volume[operation]
		kind, id, foreign := "host", "example", "other"
		if onVolume {
			kind, id, foreign = "volume", "example~data", "other~data"
		}
		t.Run(operation, func(t *testing.T) {
			full := grantRouteRights(grants, "")
			require.True(t, storageDescriptor(t, full, capability.ResourceRef{Kind: kind, ID: id}, operation))
			for _, grant := range grants {
				if !authz.IsEnvScoped(grant) {
					// An org-level grant held only on one environment does not pass the route.
					envOnly := grantRouteRights(grants, grant)
					envOnly.PerEnv["example"][grant] = struct{}{}
					require.False(t, storageDescriptor(t, envOnly, capability.ResourceRef{Kind: kind, ID: id}, operation), grant)
				}
			}
			refused := []capability.ResourceRef{{Kind: kind, ID: ""}, {Kind: kind, ID: "~data"}, {Kind: kind, ID: "example~"}, {Kind: "container", ID: id}}
			orgOnly := true
			for _, grant := range grants {
				orgOnly = orgOnly && !authz.IsEnvScoped(grant)
			}
			if orgOnly {
				// Org-level routes ignore the environment, so any well-formed host passes.
				require.True(t, storageDescriptor(t, full, capability.ResourceRef{Kind: kind, ID: foreign}, operation))
			} else {
				refused = append(refused, capability.ResourceRef{Kind: kind, ID: foreign})
			}
			for _, ref := range refused {
				require.False(t, storageDescriptor(t, full, ref, operation), fmt.Sprint(ref))
			}
			if onVolume {
				require.False(t, storageDescriptor(t, full, capability.ResourceRef{Kind: "volume", ID: "example~bind:/data"}, operation))
				require.False(t, storageDescriptor(t, full, capability.ResourceRef{Kind: "volume", ID: "example~a~b"}, operation))
			}
			for _, missing := range grants {
				partial := grantRouteRights(grants, missing)
				require.False(t, storageDescriptor(t, partial, capability.ResourceRef{Kind: kind, ID: id}, operation), missing)
			}
		})
	}
}

// grantRouteRights places env-scoped grants on environment example and
// org-level grants globally, skipping one grant when named.
func grantRouteRights(grants []string, skip string) *authz.PermissionSet {
	ps := authz.NewPermissionSet()
	ps.PerEnv["example"] = map[string]struct{}{}
	for _, grant := range grants {
		switch {
		case grant == skip:
		case authz.IsEnvScoped(grant):
			ps.PerEnv["example"][grant] = struct{}{}
		default:
			ps.Global[grant] = struct{}{}
		}
	}
	return ps
}

func TestBackupTargetCapabilitiesRequireGlobalAdminOnHub(t *testing.T) {
	admin := authz.NewPermissionSet()
	for _, permission := range authz.AllPermissions() {
		admin.Global[permission] = struct{}{}
	}
	scopedOnly := authz.NewPermissionSet()
	scopedOnly.Global[authz.PermSystemBackupsRead] = struct{}{}
	scopedOnly.Global[authz.PermSystemBackupsManage] = struct{}{}
	scopedOnly.Global[authz.PermSystemBackupsRestore] = struct{}{}
	hub := capability.ResourceRef{Kind: "hub", ID: "0"}
	for _, operation := range []string{"listBackupTargets", "createBackupTarget", "updateBackupTarget", "deleteBackupTarget", "testBackupTarget", "getBackupRecoveryKey"} {
		require.True(t, storageDescriptor(t, admin, hub, operation), operation)
		require.False(t, storageDescriptor(t, scopedOnly, hub, operation), operation)
		require.False(t, storageDescriptor(t, admin, capability.ResourceRef{Kind: "hub", ID: "1"}, operation), operation)
	}
	lister := authz.NewPermissionSet()
	lister.PerEnv["example"] = map[string]struct{}{authz.PermVolumesList: {}}
	require.True(t, storageDescriptor(t, lister, hub, "listBackupOverviews"))
	require.False(t, storageDescriptor(t, authz.NewPermissionSet(), hub, "listBackupOverviews"))
	for _, operation := range []string{"runBackup", "restoreBackup", "uploadRestore", "attachLocalBackupRecoveryKey"} {
		require.False(t, storageDescriptor(t, admin, hub, operation), operation)
	}
}
