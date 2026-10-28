// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/browser/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; Rustic-only snapshot consumers; legacy rows refuse before repository effects.

package browser

import (
	"context"
	"fmt"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/backupbrowser"
)

// Dependencies are the backup storage and repository operations the browser reads through.
type Dependencies struct {
	Docker       *docker.DockerClientService
	Engine       *backup.Engine
	Run          func(ctx context.Context, backupID string) (*volume.Backup, error)
	Repository   func(ctx context.Context, dockerClient *client.Client, entry *volume.Backup) (backup.Repository, string, error)
	Password     func(ctx context.Context, dockerClient *client.Client, repositories ...backup.Repository) (string, error)
	SanitizePath func(input string) (string, error)
}

// Service lists and browses the files stored in volume backups.
type Service struct {
	deps Dependencies
}

func NewService(deps Dependencies) *Service {
	return &Service{deps: deps}
}

func (s *Service) ListBackupFiles(ctx context.Context, backupID string) ([]string, error) {
	entry, err := s.deps.Run(ctx, backupID)
	if err != nil {
		return nil, err
	}
	if entry.Format != volume.BackupFormatRustic {
		return nil, backuptypes.NewProblem(404, "volume.backup.snapshot-not-found", nil)
	}
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	repository, snapshotID, err := s.deps.Repository(ctx, dockerClient, entry)
	if err != nil {
		return nil, err
	}
	password, err := s.deps.Password(ctx, dockerClient, repository)
	if err != nil {
		return nil, err
	}
	return s.deps.Engine.ListSnapshotFiles(ctx, dockerClient, repository, password, snapshotID, "", true)
}

// BrowseBackupFiles returns one lazy-loaded page from a volume backup tree.
func (s *Service) BrowseBackupFiles(ctx context.Context, backupID, requestedPath string, params pagination.QueryParams) ([]backuptypes.BackupFileEntry, pagination.Response, error) {
	listPath, err := backupbrowser.NormalizePath(requestedPath, true)
	recursive := false
	if err != nil || params.Start < 0 {
		return nil, pagination.Response{}, fmt.Errorf("%w: %w", common.ErrInvalidBackupSelection, err)
	}
	entry, err := s.deps.Run(ctx, backupID)
	if err != nil {
		return nil, pagination.Response{}, err
	}
	entries, err := s.backupFileEntriesInternal(ctx, entry, listPath, recursive)
	if err != nil {
		return nil, pagination.Response{}, err
	}
	items, page := backupbrowser.Browse(entries, params)
	return items, page, nil
}

func (s *Service) backupFileEntriesInternal(ctx context.Context, entry *volume.Backup, browsePath string, recursive bool) ([]backuptypes.BackupFileEntry, error) {
	if entry.Format != volume.BackupFormatRustic {
		return nil, backuptypes.NewProblem(404, "volume.backup.snapshot-not-found", nil)
	}
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	repository, snapshotID, err := s.deps.Repository(ctx, dockerClient, entry)
	if err != nil {
		return nil, err
	}
	password, err := s.deps.Password(ctx, dockerClient, repository)
	if err != nil {
		return nil, err
	}
	listed, err := s.deps.Engine.ListSnapshotFiles(ctx, dockerClient, repository, password, snapshotID, browsePath+"/", recursive)
	if err != nil {
		return nil, err
	}
	return backupbrowser.BuildEntries(listed, browsePath, recursive), nil
}

func (s *Service) BackupHasPath(ctx context.Context, backupID, filePath string) (bool, error) {
	cleaned, err := s.deps.SanitizePath(filePath)
	if err != nil {
		return false, err
	}

	entry, err := s.deps.Run(ctx, backupID)
	if err != nil {
		return false, err
	}
	if entry.Format != volume.BackupFormatRustic {
		return false, backuptypes.NewProblem(404, "volume.backup.snapshot-not-found", nil)
	}
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return false, err
	}
	repository, snapshot, err := s.deps.Repository(ctx, cli, entry)
	if err != nil {
		return false, err
	}
	password, err := s.deps.Password(ctx, cli, repository)
	if err != nil {
		return false, err
	}
	return s.deps.Engine.SnapshotHasPath(ctx, cli, repository, password, snapshot, cleaned)
}
