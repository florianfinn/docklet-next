// Source: docklet-next/docs/design/core-adoption.md
// Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Original issue #8 persistent-model registry. Durable run projection receipt model.

package schema

import (
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image/children/patch"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/system/children/upgrade"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability/children/query"
	"github.com/florianfinn/docklet-next/backend/internal/webhook"
)

// Models is the persistent schema owned by the retained core modules.
func Models() []any {
	return append(activity.Models(),
		&apikey.ApiKey{}, &backup.SystemBackupRecoveryConfig{},
		&environment.Environment{}, &event.Event{},
		&patch.ImagePatchRecord{}, &imageupdate.ImageUpdateRecord{},
		&kv.KVEntry{}, &notification.NotificationSettings{},
		&project.Project{}, &project.ProjectTag{},
		&registry.ContainerRegistry{}, &guard.Protection{},
		&role.Role{}, &role.UserRoleAssignment{}, &role.ApiKeyPermission{},
		&session.UserSession{}, &settings.SettingVariable{},
		&system.SystemBackupRun{}, &system.SystemBackupPolicy{},
		&upgrade.EnvironmentUpdateJob{}, &updater.AutoUpdateRecord{},
		&user.User{}, &user.UserAvatar{},
		&volume.VolumeBackup{}, &policies.VolumeBackupPolicy{},
		&vulnerability.VulnerabilityScanRecord{}, &vulnerability.VulnerabilityReportRecord{},
		&vulnerability.VulnerabilityScanItem{}, &vulnerability.VulnerabilityThreatIntel{},
		&vulnerability.VulnerabilityIgnore{}, &query.VulnerabilityRiskSnapshot{},
		&webhook.Webhook{},
	)
}
