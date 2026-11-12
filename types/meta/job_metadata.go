// Source: getarcaneapp/arcane/types/meta/job_metadata.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed upstream analytics heartbeat; M18 live scheduling, policy and contract foundation.

package meta

import (
	"time"

	"github.com/florianfinn/docklet-next/types/jobschedule"
)

type JobMetadata struct {
	ID             string
	Name           string
	Description    string
	Category       string
	SettingsKey    string
	EnabledKey     string
	ManagerOnly    bool
	IsContinuous   bool
	CanRunManually bool
	Prerequisites  []JobPrerequisiteMetadata
}

type JobPrerequisiteMetadata struct {
	SettingKey  string
	Label       string
	SettingsURL string
}

var jobMetadataRegistry = map[string]JobMetadata{
	"environment-health": {
		ID:             "environment-health",
		Name:           "Environment Health",
		Description:    "Checks the health and connectivity of all enabled environments",
		Category:       "monitoring",
		SettingsKey:    "environmentHealthInterval",
		ManagerOnly:    true,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites:  []JobPrerequisiteMetadata{},
	},
	"docker-client-refresh": {
		ID:             "docker-client-refresh",
		Name:           "Docker Client Refresh",
		Description:    "Refreshes the cached Docker client API version after daemon restarts or upgrades",
		Category:       "monitoring",
		SettingsKey:    "dockerClientRefreshInterval",
		ManagerOnly:    false,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites:  []JobPrerequisiteMetadata{},
	},
	"event-cleanup": {
		ID:             "event-cleanup",
		Name:           "Event Cleanup",
		Description:    "Removes old system events to maintain database performance",
		Category:       "maintenance",
		SettingsKey:    "eventCleanupInterval",
		ManagerOnly:    false,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites:  []JobPrerequisiteMetadata{},
	},
	"upgrade-log-cleanup": {
		ID:             "upgrade-log-cleanup",
		Name:           "Upgrade Log Cleanup",
		Description:    "Removes upgrade logs older than the configured retention period",
		Category:       "maintenance",
		ManagerOnly:    false,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites:  []JobPrerequisiteMetadata{},
	},
	"expired-sessions-cleanup": {
		ID:             "expired-sessions-cleanup",
		Name:           "Expired Sessions Cleanup",
		Description:    "Removes expired and old revoked user sessions from the database",
		Category:       "maintenance",
		SettingsKey:    "expiredSessionsCleanupInterval",
		ManagerOnly:    false,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites:  []JobPrerequisiteMetadata{},
	},
	"update-check": {ID: "update-check", Name: "Update check", Description: "Checks every enabled host for stack and standalone container updates", Category: "updates", ManagerOnly: true, CanRunManually: true, Prerequisites: []JobPrerequisiteMetadata{}},
	"scheduled-prune": {
		ID:             "scheduled-prune",
		Name:           "Scheduled Prune",
		Description:    "Removes unused containers, images, volumes, and networks",
		Category:       "maintenance",
		SettingsKey:    "scheduledPruneInterval",
		EnabledKey:     "scheduledPruneEnabled",
		ManagerOnly:    false,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites: []JobPrerequisiteMetadata{
			{
				SettingKey:  "scheduledPruneEnabled",
				Label:       "Scheduled prune enabled",
				SettingsURL: "/settings/general",
			},
		},
	},
	"filesystem-watcher": {
		ID:             "filesystem-watcher",
		Name:           "Filesystem Watcher",
		Description:    "Monitors project directory for changes and syncs automatically",
		Category:       "sync",
		SettingsKey:    "",
		ManagerOnly:    false,
		IsContinuous:   true,
		CanRunManually: false,
		Prerequisites:  []JobPrerequisiteMetadata{},
	},
	"vulnerability-scan": {
		ID:             "vulnerability-scan",
		Name:           "Vulnerability Scan",
		Description:    "Scans all Docker images for known vulnerabilities using Trivy",
		Category:       "security",
		SettingsKey:    "",
		ManagerOnly:    true,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites: []JobPrerequisiteMetadata{
			{
				SettingKey:  "featureVulnerabilityManagementEnabled",
				Label:       "Vulnerability management enabled",
				SettingsURL: "/settings/security",
			},
		},
	},
	"vulnerability-intel": {
		ID:             "vulnerability-intel",
		Name:           "Vulnerability Intelligence",
		Description:    "Refreshes cached CISA KEV and FIRST EPSS intelligence daily without scanning images",
		Category:       "security",
		SettingsKey:    "",
		ManagerOnly:    false,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites:  []JobPrerequisiteMetadata{},
	},
	"auto-patch": {
		ID:             "auto-patch",
		Name:           "Image Auto Patch",
		Description:    "Patches images whose latest vulnerability scan found fixable OS package vulnerabilities using Copacetic",
		Category:       "security",
		SettingsKey:    "imageAutoPatchInterval",
		EnabledKey:     "imageAutoPatchEnabled",
		ManagerOnly:    false,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites: []JobPrerequisiteMetadata{
			{
				SettingKey:  "imageAutoPatchEnabled",
				Label:       "Scheduled image patching enabled",
				SettingsURL: "/settings/security",
			},
		},
	},
	"auto-heal": {
		ID:             "auto-heal",
		Name:           "Auto Heal",
		Description:    "Automatically restarts containers that become unhealthy",
		Category:       "monitoring",
		SettingsKey:    "autoHealInterval",
		EnabledKey:     "autoHealEnabled",
		ManagerOnly:    false,
		IsContinuous:   false,
		CanRunManually: true,
		Prerequisites: []JobPrerequisiteMetadata{
			{
				SettingKey:  "autoHealEnabled",
				Label:       "Auto heal enabled",
				SettingsURL: "/settings/general",
			},
		},
	},
}

func GetJobMetadata(jobID string) (JobMetadata, bool) {
	meta, ok := jobMetadataRegistry[jobID]
	return meta, ok
}

func GetAllJobMetadata() map[string]JobMetadata {
	return jobMetadataRegistry
}

func (meta JobMetadata) ToJobStatus(schedule string, nextRun *time.Time, enabled bool, prerequisites []jobschedule.JobPrerequisite) jobschedule.JobStatus {
	return jobschedule.JobStatus{
		ID:             meta.ID,
		Name:           meta.Name,
		Description:    meta.Description,
		Category:       meta.Category,
		Schedule:       schedule,
		NextRun:        nextRun,
		Enabled:        enabled,
		ManagerOnly:    meta.ManagerOnly,
		IsContinuous:   meta.IsContinuous,
		CanRunManually: meta.CanRunManually,
		Prerequisites:  prerequisites,
		SettingsKey:    meta.SettingsKey,
	}
}
