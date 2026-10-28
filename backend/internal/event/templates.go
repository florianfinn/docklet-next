// Source: getarcaneapp/arcane/backend/internal/event/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extracted event templates; key-free backup recovery-key audit event.

package event

type eventDefinition struct {
	TitleFormat       string
	DescriptionFormat string
	Severity          EventSeverity
}

var eventDefinitions = map[EventType]eventDefinition{
	EventTypeVolumeBackupTargetKeyViewed: {"Backup recovery key viewed: %s", "The recovery key was viewed for backup target '%s'", EventSeverityInfo},
	EventTypeContainerStart:              {"Container started: %s", "Container '%s' has been started", EventSeveritySuccess},
	EventTypeContainerStop:               {"Container stopped: %s", "Container '%s' has been stopped", EventSeverityInfo},
	EventTypeContainerRestart:            {"Container restarted: %s", "Container '%s' has been restarted", EventSeverityInfo},
	EventTypeContainerDelete:             {"Container deleted: %s", "Container '%s' has been deleted", EventSeverityWarning},
	EventTypeContainerCreate:             {"Container created: %s", "Container '%s' has been created", EventSeveritySuccess},
	EventTypeContainerScan:               {"Container scanned: %s", "Security scan completed for container '%s'", EventSeverityInfo},
	EventTypeContainerUpdate:             {"Container updated: %s", "Container '%s' has been updated", EventSeverityInfo},
	EventTypeContainerError:              {"Container error: %s", "An error occurred with container '%s'", EventSeverityError},

	EventTypeImagePull:   {"Image pulled: %s", "Image '%s' has been pulled", EventSeveritySuccess},
	EventTypeImageLoad:   {"Image loaded: %s", "Image '%s' has been loaded from archive", EventSeveritySuccess},
	EventTypeImageDelete: {"Image deleted: %s", "Image '%s' has been deleted", EventSeverityWarning},
	EventTypeImageScan:   {"Image scanned: %s", "Security scan completed for image '%s'", EventSeverityInfo},
	EventTypeImageError:  {"Image error: %s", "An error occurred with image '%s'", EventSeverityError},

	EventTypeProjectDeploy: {"Project deployed: %s", "Project '%s' has been deployed", EventSeveritySuccess},
	EventTypeProjectDelete: {"Project deleted: %s", "Project '%s' has been deleted", EventSeverityWarning},
	EventTypeProjectStart:  {"Project started: %s", "Project '%s' has been started", EventSeveritySuccess},
	EventTypeProjectStop:   {"Project stopped: %s", "Project '%s' has been stopped", EventSeverityInfo},
	EventTypeProjectCreate: {"Project created: %s", "Project '%s' has been created", EventSeveritySuccess},
	EventTypeProjectUpdate: {"Project updated: %s", "Project '%s' has been updated", EventSeverityInfo},
	EventTypeProjectError:  {"Project error: %s", "An error occurred with project '%s'", EventSeverityError},

	EventTypeVolumeCreate:             {"Volume created: %s", "Volume '%s' has been created", EventSeveritySuccess},
	EventTypeVolumeRename:             {"Volume renamed: %s", "Volume '%s' has been renamed", EventSeveritySuccess},
	EventTypeVolumeDelete:             {"Volume deleted: %s", "Volume '%s' has been deleted", EventSeverityWarning},
	EventTypeVolumeError:              {"Volume error: %s", "An error occurred with volume '%s'", EventSeverityError},
	EventTypeVolumeFileCreate:         {"Volume file created: %s", "A file or directory was created in volume '%s'", EventSeveritySuccess},
	EventTypeVolumeFileDelete:         {"Volume file deleted: %s", "A file or directory was deleted in volume '%s'", EventSeverityWarning},
	EventTypeVolumeFileUpload:         {"Volume file uploaded: %s", "A file was uploaded to volume '%s'", EventSeveritySuccess},
	EventTypeVolumeFileUpdate:         {"Volume workspace updated: %s", "Files in volume '%s' were updated", EventSeverityInfo},
	EventTypeVolumeWorkspaceUpdate:    {"Volume workspace updated: %s", "Workspace files in volume '%s' were updated", EventSeverityInfo},
	EventTypeVolumeBackupCreate:       {"Volume backup created: %s", "A backup was created for volume '%s'", EventSeveritySuccess},
	EventTypeVolumeBackupDelete:       {"Volume backup deleted: %s", "A backup was deleted for volume '%s'", EventSeverityWarning},
	EventTypeVolumeBackupRestore:      {"Volume backup restored: %s", "A backup was restored for volume '%s'", EventSeverityWarning},
	EventTypeVolumeBackupRestoreFiles: {"Volume backup files restored: %s", "Selected files were restored for volume '%s'", EventSeverityWarning},
	EventTypeVolumeBackupDownload:     {"Volume backup downloaded: %s", "A backup was downloaded for volume '%s'", EventSeverityInfo},

	EventTypeNetworkCreate:     {"Network created: %s", "Network '%s' has been created", EventSeveritySuccess},
	EventTypeNetworkDelete:     {"Network deleted: %s", "Network '%s' has been deleted", EventSeverityWarning},
	EventTypeNetworkConnect:    {"Network connected: %s", "A container has been connected to network '%s'", EventSeveritySuccess},
	EventTypeNetworkDisconnect: {"Network disconnected: %s", "A container has been disconnected from network '%s'", EventSeverityInfo},
	EventTypeNetworkError:      {"Network error: %s", "An error occurred with network '%s'", EventSeverityError},

	EventTypeSystemPrune:      {"System prune completed", "System resources have been pruned", EventSeverityInfo},
	EventTypeSystemAutoUpdate: {"System auto-update completed", "System auto-update process has completed", EventSeverityInfo},
	EventTypeSystemUpgrade:    {"System upgrade completed", "System upgrade process has completed", EventSeverityInfo},

	EventTypeUserLogin:  {"User logged in: %s", "User '%s' has logged in", EventSeverityInfo},
	EventTypeUserLogout: {"User logged out: %s", "User '%s' has logged out", EventSeverityInfo},
}
