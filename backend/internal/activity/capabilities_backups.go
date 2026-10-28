// Source: docklet-next/docs/design/volume-backups.md
// Revision: 7139e2bc64162c47b9f027ecbcaf1b06604c97c9
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original backup, restore and backup target descriptor rights for #284.

package activity

import (
	"strings"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
)

var backupOperationIDs = []string{
	"setBackupPolicy", "runBackup", "restoreBackup", "restoreBackupFiles", "browseBackup",
	"downloadBackup", "deleteBackup", "uploadRestore", "getBackupOverview", "listBackupOverviews",
	"getBackupRepository", "checkBackupRepository", "listBackupTargets", "createBackupTarget",
	"updateBackupTarget", "deleteBackupTarget", "testBackupTarget", "getBackupTargetReferences",
	"getBackupRecoveryKey", "attachLocalBackupRecoveryKey",
}

// Each grant mirrors the permission its route registers. Volume refs are
// env~name for named volumes; host refs are an environment id.
var backupVolumeGrants = map[string][]string{
	"setBackupPolicy": {authz.PermVolumesBackup}, "runBackup": {authz.PermVolumesBackup},
	"restoreBackup": {authz.PermVolumesBackup}, "restoreBackupFiles": {authz.PermVolumesBackup},
	"deleteBackup": {authz.PermVolumesBackup}, "browseBackup": {authz.PermVolumesRead},
	"downloadBackup": {authz.PermVolumesRead}, "uploadRestore": {authz.PermVolumesBackup, authz.PermVolumesUpload},
}

// Host-scoped target routes test and read the built-in local target; its key
// routes are system-restore rights on the hub.
var backupHostGrants = map[string][]string{
	"getBackupOverview": {authz.PermVolumesList}, "getBackupRepository": {authz.PermVolumesRead},
	"checkBackupRepository": {authz.PermVolumesBackup}, "getBackupTargetReferences": {authz.PermVolumesRead},
	"testBackupTarget": {authz.PermVolumesBackup}, "getBackupRecoveryKey": {authz.PermSystemBackupsRestore},
	"attachLocalBackupRecoveryKey": {authz.PermSystemBackupsRestore},
}

// Stored targets live behind global-admin hub routes.
var backupHubAdminOperations = map[string]bool{
	"listBackupTargets": true, "createBackupTarget": true, "updateBackupTarget": true,
	"deleteBackupTarget": true, "testBackupTarget": true, "getBackupRecoveryKey": true,
}

func backupCapabilityAllowed(operation string, ref capability.ResourceRef, ps *authz.PermissionSet) bool {
	if ref.Kind == "hub" && ref.ID == "0" {
		if backupHubAdminOperations[operation] {
			return ps.IsGlobalAdmin()
		}
		if operation == "listBackupOverviews" {
			return ps.AllowsAny(authz.PermVolumesList)
		}
		return false
	}
	env, required := "", []string(nil)
	switch {
	case ref.Kind == "host" && ref.ID != "" && !strings.Contains(ref.ID, "~") && backupHostGrants[operation] != nil:
		env, required = ref.ID, backupHostGrants[operation]
	case ref.Kind == "volume" && backupVolumeGrants[operation] != nil:
		scoped, raw, ok := strings.Cut(ref.ID, "~")
		if !ok || scoped == "" || raw == "" || strings.Contains(raw, "~") || strings.HasPrefix(raw, "bind:") {
			return false
		}
		env, required = scoped, backupVolumeGrants[operation]
	default:
		return false
	}
	for _, permission := range required {
		// Org-level grants are checked without an environment, as RequirePermission does.
		scope := env
		if !authz.IsEnvScoped(permission) {
			scope = ""
		}
		if !ps.Allows(permission, scope) {
			return false
		}
	}
	return true
}
