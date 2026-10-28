// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/restore/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M12 persisted volume lifecycle and legacy restore cut.

package restore

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/backupbrowser"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"path"
	"strings"
)

type Dependencies struct {
	Docker      *docker.DockerClientService
	Engine      *backup.Engine
	Run         func(context.Context, string) (*volume.Backup, error)
	Repository  func(context.Context, *client.Client, *volume.Backup) (backup.Repository, string, error)
	Password    func(context.Context, *client.Client, ...backup.Repository) (string, error)
	HelperImage func(context.Context, *client.Client) (string, error)
	Exec        func(context.Context, string, string, []string) (string, string, error)
}
type Service struct {
	deps      Dependencies
	lifecycle *Lifecycle
}

func NewService(deps Dependencies) *Service  { return &Service{deps: deps} }
func (s *Service) SetLifecycle(l *Lifecycle) { s.lifecycle = l }

type selectionInternal struct {
	entries    []backuptypes.BackupFileEntry
	globalRoot bool
}

func (
	s *Service,
) resolveRestoreSelectionInternal(
	ctx context.Context,
	dockerClient *client.Client,
	entry *volume.Backup,
	repository backup.Repository,
	snapshotID string,
	selection backuptypes.RestoreSelection,
) (
	selectionInternal,
	error,
) {
	if selection.SelectAll && strings.TrimSpace(selection.Search) == "" {
		if _, err := backupbrowser.NormalizeSelection(selection, []backuptypes.BackupFileEntry{{Path: "", IsDirectory: true}}); err != nil {
			return selectionInternal{}, fmt.Errorf("%w: %w", common.ErrInvalidBackupSelection, err)
		}
		return selectionInternal{globalRoot: true}, nil
	}
	resolved := selectionInternal{}
	var eligible []backuptypes.BackupFileEntry
	var err error
	var listed []string
	password, passwordErr := s.deps.Password(ctx, dockerClient, repository)
	if passwordErr != nil {
		return selectionInternal{}, passwordErr
	}
	listed, err = s.deps.Engine.ListSnapshotFiles(ctx, dockerClient, repository, password, snapshotID, "", true)
	eligible = backupbrowser.BuildEntries(listed, "", true)

	if err != nil {
		return selectionInternal{}, err
	}
	resolved.entries, err = backupbrowser.NormalizeSelection(selection, eligible)
	if err != nil {
		return selectionInternal{}, fmt.Errorf("%w: %w", common.ErrInvalidBackupSelection, err)
	}
	return resolved, nil
}

func (
	s *Service,
) restoreSelectionInternal(
	ctx context.Context,
	dockerClient *client.Client,
	volumeName, backupID string,
	entry *volume.Backup,
	repository backup.Repository,
	snapshotID string,
	selection selectionInternal,
) error {
	password, err := s.deps.Password(ctx, dockerClient, repository)
	if err != nil {
		return err
	}
	target := mount.Mount{Type: mount.TypeVolume, Source: volumeName, Target: "/volume"}
	if selection.globalRoot {
		if restoreSnapshotErr := s.deps.Engine.RestoreSnapshot(ctx, dockerClient, repository, password, snapshotID, target, backup.RestoreOptions{DeleteExtra: true}); restoreSnapshotErr != nil {
			return fmt.Errorf("failed to restore Rustic snapshot: %w", restoreSnapshotErr)
		}
		return nil
	}
	for _, selectedEntry := range selection.entries {
		sourcePath := selectedEntry.Path
		if selectedEntry.IsDirectory {
			sourcePath += "/"
		}
		options := backup.RestoreOptions{DeleteExtra: selectedEntry.IsDirectory, SourcePath: sourcePath, DestinationPath: path.Join(target.Target, selectedEntry.Path)}
		if restoreSelectedEntryErr := s.deps.Engine.RestoreSnapshot(ctx, dockerClient, repository, password, snapshotID, target, options); restoreSelectedEntryErr != nil {
			return fmt.Errorf("failed to restore %s from Rustic snapshot: %w", selectedEntry.Path, restoreSelectedEntryErr)
		}
	}
	return nil
}
