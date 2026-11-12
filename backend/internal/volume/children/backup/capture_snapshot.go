// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; M12 backup foundation; M12 persisted volume lifecycle and legacy restore cut.; Remove legacy archive and centralized volume execution; preserve historical metadata deletion and safety pins; notification and Auto Heal contracts and delivery for #296; M16 bind mounts and backup completion.

package backup

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
)

func (s *Service) createLocalBackupSnapshotInternal(ctx context.Context, dockerClient *client.Client, entry *volume.Backup) error {
	repository, repoErr := s.repositoryFor(ctx, dockerClient, string(entry.Destination), false)
	if repoErr != nil {
		return repoErr
	}
	password, passwordErr := s.volumeBackupPasswordInternal(ctx, dockerClient, repository)
	if passwordErr != nil {
		return passwordErr
	}
	if err := jobcontext.Progress(ctx, scheduler.TargetOutcome{ResourceType: "backup_destination", ID: entry.ID + ":local", Status: scheduler.Running}); err != nil {
		return err
	}
	sourceMount := volumeSourceMountInternal(entry.VolumeName)
	if volume.IsBindKey(entry.VolumeName) {
		source, err := bindpath.Check(ctx, dockerClient, s.deps.DB, entry.VolumeName)
		if err != nil {
			return err
		}
		image, err := s.deps.HelperImage(ctx, dockerClient)
		if err != nil {
			return err
		}
		kind, err := bindpath.Witness(ctx, dockerClient, image, source, s.deps.Exec)
		if err != nil {
			return err
		}
		sourceMount = bindpath.SourceMount(source, kind, true)
	}
	input := backup.RootSnapshotInput(sourceMount, backup.RunSnapshotTag(entry.ID))
	input.Sources = []string{"/volume"}
	localSnapshot, err := s.deps.Engine.CreateSnapshot(
		ctx,
		dockerClient,
		repository,
		password,
		entry.VolumeName,
		input,
	)
	if err != nil {
		return fmt.Errorf("failed to create local Rustic snapshot: %w", err)
	}
	entry.LocalSnapshotID = localSnapshot.ID
	entry.Size = localSnapshot.Size
	if saveLocalBackupErr := s.deps.Store.Save(ctx, entry); saveLocalBackupErr != nil {
		return saveLocalBackupErr
	}
	if progressErr := jobcontext.Progress(
		ctx,
		scheduler.TargetOutcome{
			ResourceType: "backup_destination",
			ID:           entry.ID + ":local",
			Status:       scheduler.Succeeded,
			Message:      localSnapshot.ID,
		},
	); progressErr != nil {
		return progressErr
	}
	return nil
}
