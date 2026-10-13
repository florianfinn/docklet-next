// Source: getarcaneapp/arcane/backend/pkg/authz/catalog.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Functional cut: removed template catalogue; functional cut: removed federated credentials; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm.

package authz

const (
	PermissionScopeGlobal = "global"
	PermissionScopeEnv    = "env"
)

// PermissionCatalogResource describes a resource group in the permission
// catalog. It is the authz-owned source for permission ordering, scope, and
// display metadata used by API manifests and validation helpers.
type PermissionCatalogResource struct {
	Key     string
	Label   string
	Scope   string
	Actions []PermissionCatalogAction
}

// PermissionCatalogAction describes one recognized permission.
type PermissionCatalogAction struct {
	Key         string
	Permission  string
	Label       string
	Description string
}

var permissionCatalog = []PermissionCatalogResource{
	{"users", "Users", PermissionScopeGlobal, []PermissionCatalogAction{
		{"list", PermUsersList, "List", ""},
		{"read", PermUsersRead, "Read", ""},
		{"create", PermUsersCreate, "Create", ""},
		{"update", PermUsersUpdate, "Update", ""},
		{"delete", PermUsersDelete, "Delete", ""},
	}},
	{"roles", "Roles", PermissionScopeGlobal, []PermissionCatalogAction{
		{"list", PermRolesList, "List", ""},
		{"read", PermRolesRead, "Read", ""},
	}},
	{"apikeys", "API Keys", PermissionScopeGlobal, []PermissionCatalogAction{
		{"list", PermApiKeysList, "List", ""},
		{"read", PermApiKeysRead, "Read", ""},
		{"create", PermApiKeysCreate, "Create", ""},
		{"update", PermApiKeysUpdate, "Update", ""},
		{"delete", PermApiKeysDelete, "Delete", ""},
	}},
	{"settings", "Settings", PermissionScopeGlobal, []PermissionCatalogAction{
		{"read", PermSettingsRead, "Read", ""},
		{"write", PermSettingsWrite, "Write", ""},
	}},
	{"environments", "Environments", PermissionScopeGlobal, []PermissionCatalogAction{
		{"list", PermEnvironmentsList, "List", ""},
		{"read", PermEnvironmentsRead, "Read", ""},
		{"create", PermEnvironmentsCreate, "Create", ""},
		{"update", PermEnvironmentsUpdate, "Update", ""},
		{"delete", PermEnvironmentsDelete, "Delete", ""},
		{"pair", PermEnvironmentsPair, "Pair agent", ""},
		{"sync", PermEnvironmentsSync, "Sync heartbeat", ""},
	}},
	{"registries", "Container Registries", PermissionScopeGlobal, []PermissionCatalogAction{
		{"list", PermRegistriesList, "List", ""},
		{"read", PermRegistriesRead, "Read", ""},
		{"create", PermRegistriesCreate, "Create", ""},
		{"update", PermRegistriesUpdate, "Update", ""},
		{"delete", PermRegistriesDelete, "Delete", ""},
		{"test", PermRegistriesTest, "Test", ""},
		{"browse", PermRegistriesBrowse, "Browse", "Browse repositories and tags stored in the registry (the pages also require Read)"},
		{"delete-tags", PermRegistriesDeleteTags, "Delete tags", "Delete image manifests from the registry"},
	}},
	{"s3-destinations", "S3 Destinations", PermissionScopeGlobal, []PermissionCatalogAction{
		{"list", PermS3DestinationsList, "List", ""},
		{"read", PermS3DestinationsRead, "Read", ""},
		{"create", PermS3DestinationsCreate, "Create", ""},
		{"update", PermS3DestinationsUpdate, "Update", ""},
		{"delete", PermS3DestinationsDelete, "Delete", ""},
		{"test", PermS3DestinationsTest, "Test", ""},
		{"sync", PermS3DestinationsSync, "Sync", ""},
	}},
	{"system-backups", "System Backups", PermissionScopeGlobal, []PermissionCatalogAction{
		{"read", PermSystemBackupsRead, "Read", ""},
		{"manage", PermSystemBackupsManage, "Manage", ""},
		{"restore", PermSystemBackupsRestore, "Restore", ""},
		{"recovery-key", PermSystemBackupsRecoveryKey, "Recovery Key", ""},
	}},
	{"events", "Events", PermissionScopeGlobal, []PermissionCatalogAction{
		{"read", PermEventsRead, "Read", ""},
		{"delete", PermEventsDelete, "Delete", ""},
	}},
	{"customize", "Customize", PermissionScopeGlobal, []PermissionCatalogAction{
		{"manage", PermCustomizeManage, "Manage", ""},
	}},
	{"diagnostics", "Diagnostics", PermissionScopeGlobal, []PermissionCatalogAction{
		{"read", PermDiagnosticsRead, "View", "View runtime diagnostics, pprof profiles, and backend logs"},
	}},
	{"containers", "Containers", PermissionScopeEnv, []PermissionCatalogAction{
		{"list", PermContainersList, "List", ""},
		{"read", PermContainersRead, "Read", ""},
		{"logs", PermContainersLogs, "View logs", ""},
		{"create", PermContainersCreate, "Create", ""},
		{"start", PermContainersStart, "Start", ""},
		{"stop", PermContainersStop, "Stop", ""},
		{"restart", PermContainersRestart, "Restart", ""},
		{"redeploy", PermContainersRedeploy, "Redeploy", ""},
		{"edit", PermContainersEdit, "Edit (recreate with changes)", ""},
		{"kill", PermContainersKill, "Kill (send signal)", ""},
		{"pause", PermContainersPause, "Pause / unpause", ""},
		{"delete", PermContainersDelete, "Delete", ""},
		{"exec", PermContainersExec, "Exec / terminal", ""},
		{"autoupdate", PermContainersAutoUpdate, "Auto-update", ""},
	}},
	{"projects", "Projects", PermissionScopeEnv, []PermissionCatalogAction{
		{"list", PermProjectsList, "List", ""},
		{"read", PermProjectsRead, "Read", ""},
		{"logs", PermProjectsLogs, "View logs", ""},
		{"create", PermProjectsCreate, "Create", ""},
		{"update", PermProjectsUpdate, "Update", ""},
		{"deploy", PermProjectsDeploy, "Deploy", ""},
		{"down", PermProjectsDown, "Bring down", ""},
		{"restart", PermProjectsRestart, "Restart", ""},
		{"delete", PermProjectsDelete, "Delete", ""},
		{"archive", PermProjectsArchive, "Archive / unarchive", ""},
	}},
	{"images", "Images", PermissionScopeEnv, []PermissionCatalogAction{
		{"list", PermImagesList, "List", ""},
		{"read", PermImagesRead, "Read", ""},
		{"pull", PermImagesPull, "Pull", ""},
		{"push", PermImagesPush, "Push", ""},
		{"build", PermImagesBuild, "Build", ""},
		{"tag", PermImagesTag, "Tag", ""},
		{"commit", PermImagesCommit, "Commit container", ""},
		{"prune", PermImagesPrune, "Prune", ""},
		{"delete", PermImagesDelete, "Delete", ""},
		{"upload", PermImagesUpload, "Upload", ""},
		{"patch", PermImagesPatch, "Patch", ""},
	}},
	{"volumes", "Volumes", PermissionScopeEnv, []PermissionCatalogAction{
		{"list", PermVolumesList, "List", ""},
		{"read", PermVolumesRead, "Read", ""},
		{"create", PermVolumesCreate, "Create", ""},
		{"rename", PermVolumesRename, "Rename", ""},
		{"delete", PermVolumesDelete, "Delete", ""},
		{"prune", PermVolumesPrune, "Prune", ""},
		{"upload", PermVolumesUpload, "Upload", ""},
		{"backup", PermVolumesBackup, "Backup / restore", ""},
	}},
	{"networks", "Networks", PermissionScopeEnv, []PermissionCatalogAction{
		{"list", PermNetworksList, "List", ""},
		{"read", PermNetworksRead, "Read", ""},
		{"create", PermNetworksCreate, "Create", ""},
		{"delete", PermNetworksDelete, "Delete", ""},
		{"prune", PermNetworksPrune, "Prune", ""},
		{"connect", PermNetworksConnect, "Connect container", ""},
		{"disconnect", PermNetworksDisconnect, "Disconnect container", ""},
	}},
	{"webhooks", "Webhooks", PermissionScopeEnv, []PermissionCatalogAction{
		{"list", PermWebhooksList, "List", ""},
		{"create", PermWebhooksCreate, "Create", ""},
		{"update", PermWebhooksUpdate, "Update", ""},
		{"delete", PermWebhooksDelete, "Delete", ""},
	}},
	{"jobs", "Background Jobs", PermissionScopeEnv, []PermissionCatalogAction{
		{"manage", PermJobsManage, "Manage", ""},
	}},
	{"notifications", "Notifications", PermissionScopeGlobal, []PermissionCatalogAction{
		{"manage", PermNotificationsManage, "Manage", ""},
	}},
	{"dashboard", "Dashboard", PermissionScopeEnv, []PermissionCatalogAction{
		{"read", PermDashboardRead, "Read", ""},
	}},
	{"system", "System", PermissionScopeEnv, []PermissionCatalogAction{
		{"read", PermSystemRead, "Read", ""},
		{"prune", PermSystemPrune, "Prune", ""},
		{"upgrade", PermSystemUpgrade, "Trigger upgrade", ""},
	}},
	{"image-updates", "Image Updates", PermissionScopeEnv, []PermissionCatalogAction{
		{"read", PermImageUpdatesRead, "Read", ""},
		{"check", PermImageUpdatesCheck, "Check", ""},
	}},
	{"vulnerabilities", "Vulnerabilities", PermissionScopeEnv, []PermissionCatalogAction{
		{"read", PermVulnsRead, "Read", ""},
		{"scan", PermVulnsScan, "Scan", ""},
		{"manage", PermVulnsManage, "Manage ignores", ""},
	}},
	{"build-workspaces", "Build Workspaces", PermissionScopeEnv, []PermissionCatalogAction{
		{"manage", PermBuildWorkspacesManage, "Manage", ""},
	}},
	{"activities", "Activities", PermissionScopeEnv, []PermissionCatalogAction{
		{"read", PermActivitiesRead, "Read", ""},
		{"cancel", PermActivitiesCancel, "Cancel", ""},
		{"delete", PermActivitiesDelete, "Clear history", ""},
	}},
}

// PermissionCatalog returns a defensive copy of the full permission catalog.
func PermissionCatalog() []PermissionCatalogResource {
	out := make([]PermissionCatalogResource, len(permissionCatalog))
	for i, resource := range permissionCatalog {
		out[i] = resource
		out[i].Actions = append([]PermissionCatalogAction(nil), resource.Actions...)
	}
	return out
}
