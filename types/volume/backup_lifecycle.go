// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original volume backup lifecycle contract.

package volume

import "time"

type StoppedContainer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RestoreRequest struct {
	Mode          string `json:"mode,omitempty" enum:"replace,new" default:"replace"`
	NewVolumeName string `json:"newVolumeName,omitempty"`
}

type RestoreOperation struct {
	ActivityID string `json:"activityId"`
	RestoreID  string `json:"restoreId"`
}

type RestoreRun struct {
	RestoreID         string             `json:"restoreId"`
	ActivityID        string             `json:"activityId"`
	Kind              string             `json:"kind" enum:"full,files,upload"`
	Mode              string             `json:"mode" enum:"replace,new"`
	NewVolumeName     string             `json:"newVolumeName,omitempty"`
	Status            string             `json:"status" enum:"running,succeeded,failed,partial"`
	Phase             string             `json:"phase" enum:"preparing,writing,done"`
	Code              string             `json:"code,omitempty"`
	At                time.Time          `json:"at"`
	SafetyBackupID    string             `json:"safetyBackupId,omitempty"`
	StoppedContainers []StoppedContainer `json:"stoppedContainers"`
	PendingRestart    bool               `json:"pendingRestart"`
}

type BackupRunning struct {
	Kind       string `json:"kind" enum:"backup,restore,download,check"`
	ActivityID string `json:"activityId"`
	RestoreID  string `json:"restoreId,omitempty"`
}

type BackupVolumeOverview struct {
	Name          string         `json:"name"`
	VolumeExists  bool           `json:"volumeExists"`
	Policy        *BackupPolicy  `json:"policy,omitempty"`
	LastBackup    *BackupEntry   `json:"lastBackup,omitempty"`
	LastSuccessAt *time.Time     `json:"lastSuccessAt,omitempty"`
	Count         int            `json:"count"`
	Running       *BackupRunning `json:"running,omitempty"`
	LastRestore   *RestoreRun    `json:"lastRestore,omitempty"`
}

type BackupRepositoryCheck struct {
	OK            bool      `json:"ok"`
	Code          string    `json:"code,omitempty"`
	At            time.Time `json:"at"`
	SnapshotCount *int64    `json:"snapshotCount,omitempty" minimum:"0"`
	FreeBytes     *int64    `json:"freeBytes,omitempty" minimum:"0"`
	TotalBytes    *int64    `json:"totalBytes,omitempty" minimum:"0"`
	Initialized   bool      `json:"initialized"`
}

type BackupRepository struct {
	Mode        string                 `json:"mode" enum:"mount,volume"`
	Name        string                 `json:"name"`
	EngineImage string                 `json:"engineImage"`
	LastCheck   *BackupRepositoryCheck `json:"lastCheck,omitempty"`
	Running     *BackupRunning         `json:"running,omitempty"`
}

type BackupOverview struct {
	TimeZone               string                  `json:"timeZone"`
	Repository             BackupRepositorySummary `json:"repository"`
	Volumes                []BackupVolumeOverview  `json:"volumes"`
	BackupTargetAuthority  string                  `json:"backupTargetAuthority"`
	BackupTargetSetVersion int64                   `json:"backupTargetSetVersion"`
	Targets                []UsableBackupTarget    `json:"targets"`
}
type BackupRepositorySummary struct {
	Mode        string `json:"mode" enum:"mount,volume"`
	Available   bool   `json:"available"`
	LastCheckOK *bool  `json:"lastCheckOk,omitempty"`
}
type UsableBackupTarget struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind" enum:"local,mount,smb,nfs"`
}

type UploadRestoreRequest struct {
	UploadID      string `json:"uploadId"`
	Mode          string `json:"mode,omitempty" enum:"replace,new" default:"replace"`
	NewVolumeName string `json:"newVolumeName,omitempty"`
}
