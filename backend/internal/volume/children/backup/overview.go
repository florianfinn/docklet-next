// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original single-query host overview with bounded metadata and operation rediscovery.

package backup

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
)

// All SQL expressions are fixed identifiers owned here. No request value is
// interpolated; one snapshot reads policies, history and safe target metadata.
func overviewQuery() string {
	fields := []string{"kind", "id", "volume_name", "created_at", "status", "trigger", "destination", "format", "snapshot_unavailable_at", "local_snapshot_id", "policy_id", "activity_id", "error_code", "size", "schedule", "enabled", "retention_count", "stop_containers", "local_enabled", "target_id", "phase", "mode", "new_volume_name", "pending_restart", "stopped_containers", "safety_backup_id", "value", "host_id", "error_reason", "error"}
	selectFrom := func(table, kind string, columns []string, overrides map[string]string) string {
		selected := make(map[string]string, len(columns)+1)
		for _, column := range columns {
			selected[column] = column
		}
		selected["kind"] = "'" + kind + "'"
		for field, expression := range overrides {
			selected[field] = expression
		}
		expressions := make([]string, 0, len(fields))
		for _, field := range fields {
			expression := selected[field]
			if expression == "" {
				expression = "NULL"
			}
			expressions = append(expressions, expression+" AS "+field)
		}
		return "SELECT " + strings.Join(expressions, ",") + " FROM " + table
	}
	return strings.Join([]string{
		selectFrom("volume_backups", "backup", []string{"id", "volume_name", "created_at", "status", "trigger", "destination", "format", "snapshot_unavailable_at", "local_snapshot_id", "policy_id", "activity_id", "error_code", "size", "stop_containers", "error"}, nil),
		selectFrom("volume_backup_policies", "policy", []string{"id", "volume_name", "created_at", "schedule", "enabled", "retention_count", "stop_containers", "local_enabled", "target_id"}, nil),
		selectFrom("volume_restore_runs", "restore", []string{"id", "volume_name", "status", "activity_id", "error_code", "phase", "mode", "new_volume_name", "pending_restart", "stopped_containers", "safety_backup_id", "error_reason"}, map[string]string{"created_at": "started_at", "trigger": "kind"}),
		selectFrom("settings WHERE key IN ('volumeBackupRepositoryCheck','backup_target_authority','backup_target_set_version','backupTargetAuthority','backupTargetSetVersion','hubHostId')", "setting", []string{"value"}, map[string]string{"id": "key"}),
		selectFrom("activities WHERE resource_type = 'volume_backup_download' AND status IN ('queued','running')", "download", []string{"id", "created_at"}, map[string]string{"volume_name": "resource_id", "activity_id": "id"}),
		selectFrom("backup_targets WHERE retired_at IS NULL", "target", []string{"id", "host_id"}, map[string]string{"volume_name": "name", "trigger": "kind"}),
	}, " UNION ALL ")
}

type overviewRecord struct {
	Kind, ID, VolumeName, Status, Trigger, Destination, Format, LocalSnapshotID, PolicyID, ActivityID, ErrorCode string
	SnapshotUnavailableAt                                                                                        *time.Time
	CreatedAt                                                                                                    *time.Time
	Size                                                                                                         int64
	Schedule                                                                                                     string
	Enabled                                                                                                      bool
	RetentionCount                                                                                               int
	StopContainers, LocalEnabled                                                                                 bool
	TargetID, Phase, Mode, NewVolumeName                                                                         string
	PendingRestart                                                                                               bool
	StoppedContainers, SafetyBackupID, Value, ErrorReason, Error                                                 string
	HostID                                                                                                       *string
}

// backupInternal restores the stored backup row so the overview shares the
// entry projection of every other backup route.
func (r *overviewRecord) backupInternal() *volume.Backup {
	stored := &volume.Backup{
		ID: r.ID, VolumeName: r.VolumeName, Size: r.Size, Status: r.Status, Trigger: r.Trigger,
		Destination: volume.BackupDestination(r.Destination), Format: volume.BackupFormat(r.Format),
		SnapshotUnavailableAt: r.SnapshotUnavailableAt, LocalSnapshotID: r.LocalSnapshotID, PolicyID: r.PolicyID,
		ErrorCode: r.ErrorCode, StopContainers: r.StopContainers, Error: r.Error,
	}
	if r.CreatedAt != nil {
		stored.CreatedAt = *r.CreatedAt
	}
	if r.ActivityID != "" {
		stored.ActivityID = new(r.ActivityID)
	}
	return stored
}

func (s *Service) Overview(ctx context.Context) (volume.BackupOverview, error) {
	var rows []overviewRecord
	if err := s.deps.DB.WithContext(ctx).Raw(overviewQuery()).Scan(&rows).Error; err != nil {
		return volume.BackupOverview{}, err
	}
	result := volume.BackupOverview{TimeZone: "UTC", Volumes: []volume.BackupVolumeOverview{}, Targets: []volume.UsableBackupTarget{{ID: "local", Name: "Local", Kind: "local"}}, Repository: volume.BackupRepositorySummary{Mode: "volume", Available: true}}
	if s.deps.Location != nil {
		result.TimeZone = s.deps.Location.String()
	}
	settings := map[string]string{}
	for _, r := range rows {
		if r.Kind == "setting" {
			settings[r.ID] = r.Value
		}
	}
	host := "0"
	if settings["hubHostId"] != "" {
		host = settings["hubHostId"]
	}
	result.BackupTargetAuthority = settings["backup_target_authority"]
	version := settings["backup_target_set_version"]
	if host != "0" {
		result.BackupTargetAuthority = settings["backupTargetAuthority"]
		version = settings["backupTargetSetVersion"]
	}
	result.BackupTargetSetVersion, _ = strconv.ParseInt(version, 10, 64)
	if raw := settings["volumeBackupRepositoryCheck"]; raw != "" {
		var check volume.BackupRepositoryCheck
		if err := json.Unmarshal([]byte(raw), &check); err != nil {
			return result, dto.NewProblem(500, "volume.backup.engine-output-invalid", err)
		}
		result.Repository.LastCheckOK = &check.OK
		result.Repository.Available = check.OK
	}
	byName := map[string]*volume.BackupVolumeOverview{}
	get := func(name string) *volume.BackupVolumeOverview {
		if byName[name] == nil {
			byName[name] = &volume.BackupVolumeOverview{Name: name}
		}
		return byName[name]
	}
	// A stable newest-first order also resolves equal timestamps by immutable ID.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt == nil {
			return false
		}
		if rows[j].CreatedAt == nil {
			return true
		}
		if rows[i].CreatedAt.Equal(*rows[j].CreatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt.After(*rows[j].CreatedAt)
	})
	for _, r := range rows {
		switch r.Kind {
		case "target":
			if r.HostID == nil || *r.HostID == host {
				result.Targets = append(result.Targets, volume.UsableBackupTarget{ID: r.ID, Name: r.VolumeName, Kind: r.Trigger})
			}
		case "policy":
			policy := &volume.BackupPolicy{ID: r.ID, VolumeName: r.VolumeName, Enabled: r.Enabled, Schedule: r.Schedule, RetentionCount: r.RetentionCount, StopContainers: r.StopContainers, LocalEnabled: r.LocalEnabled, TargetID: r.TargetID}
			policy.NextRunAt = s.policies.NextRun(r.ID, r.Enabled)
			get(r.VolumeName).Policy = policy
		case "backup":
			item := get(r.VolumeName)
			item.Count++
			if item.LastBackup == nil {
				item.LastBackup = new(entryInternal(r.backupInternal()))
			}
			if item.LastSuccessAt == nil && (r.Status == "succeeded" || r.Status == "partial") && r.LocalSnapshotID != "" {
				item.LastSuccessAt = r.CreatedAt
			}
			if r.Status == "running" && item.Running == nil {
				item.Running = &volume.BackupRunning{Kind: "backup", ActivityID: r.ActivityID}
			}
		case "download":
			item := get(r.VolumeName)
			if item.Running == nil {
				item.Running = &volume.BackupRunning{Kind: "download", ActivityID: r.ActivityID}
			}
		case "restore":
			item := get(r.VolumeName)
			if item.LastRestore == nil || r.PendingRestart {
				stopped := []volume.StoppedContainer{}
				if r.StoppedContainers != "" {
					if err := json.Unmarshal([]byte(r.StoppedContainers), &stopped); err != nil {
						return result, dto.NewProblem(500, "volume.backup.engine-output-invalid", err)
					}
				}
				run := volume.RestoreRun{RestoreID: r.ID, ActivityID: r.ActivityID, Kind: r.Trigger, Mode: r.Mode, NewVolumeName: r.NewVolumeName, Status: r.Status, Phase: r.Phase, Code: r.ErrorCode, Reason: r.ErrorReason, At: *r.CreatedAt, SafetyBackupID: r.SafetyBackupID, StoppedContainers: stopped, PendingRestart: r.PendingRestart}
				if item.LastRestore == nil {
					item.LastRestore = &run
				}
				// A later restore that failed before writing does not clear earlier stopped containers.
				if r.PendingRestart {
					item.PendingRestores = append(item.PendingRestores, run)
				}
			}
			if r.Status == "running" && item.Running == nil {
				item.Running = &volume.BackupRunning{Kind: "restore", ActivityID: r.ActivityID, RestoreID: r.ID}
			}
		}
	}
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return result, dto.NewProblem(503, "volume.backup.engine-unavailable", err)
	}
	listed, err := cli.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return result, dto.NewProblem(503, "volume.backup.engine-unavailable", err)
	}
	for _, v := range listed.Items {
		if _, internal := v.Labels[libarcane.InternalResourceLabel]; !internal {
			get(v.Name).VolumeExists = true
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result.Volumes = append(result.Volumes, *byName[name])
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	if len(encoded) > dto.MaxVolumeResponseBytes {
		return result, dto.NewProblem(413, "volume.backup.listing-too-large", nil)
	}
	return result, nil
}
