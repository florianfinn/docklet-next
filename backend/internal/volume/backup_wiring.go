// Source: getarcaneapp/arcane/backend/internal/volume/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Split retained backup scheduler and repository wiring from volume service.

package volume

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// SetScheduler injects the dynamic scheduler and admission gate for per-policy
// backup jobs. Agent mode passes them too: agents run their own volume backups.
func (s *VolumeService) SetScheduler(ctx context.Context, dynamicScheduler scheduler.DynamicScheduler, admissionGate *runs.Admission) error {
	return s.backup.SetScheduler(ctx, dynamicScheduler, admissionGate)
}

func (s *VolumeService) RegisterBackupJobsOnStartup(ctx context.Context) {
	s.backup.RegisterJobsOnStartup(ctx)
}

// BackupStorageMount resolves the repository mount shared with system-backup operations.
func (s *VolumeService) BackupStorageMount(ctx context.Context, dockerClient *client.Client, target string, readOnly bool) (mount.Mount, error) {
	return s.backup.StorageMount(ctx, dockerClient, target, readOnly)
}

func (s *VolumeService) ReconcileBackup(ctx context.Context, previous scheduler.Run, volumeName string) (scheduler.Outcome, error) {
	return s.backup.ReconcileBackup(ctx, previous, volumeName)
}

// MigrateRepositoryPasswords re-keys this instance's own volume backup repositories to the recovery key; other instances re-key their own roots.
func (s *VolumeService) MigrateRepositoryPasswords(ctx context.Context) error {
	return s.backup.MigrateRepositoryPasswords(ctx)
}

func (s *VolumeService) SetBackupInputCleaner(clean func(context.Context, []string) error) {
	s.backup.SetInputCleaner(clean)
}
