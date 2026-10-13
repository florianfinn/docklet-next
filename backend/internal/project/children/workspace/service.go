// Source: getarcaneapp/arcane/backend/internal/project/children/workspace/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories.

// Package workspace reads and edits project directories as bounded,
// revision-checked workspaces.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/florianfinn/docklet-next/types/project"
	workspacetypes "github.com/florianfinn/docklet-next/types/workspace"
	"go.getarcane.app/acfs"
	"go.getarcane.app/acfs/types"
	"go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	acfsutils "github.com/florianfinn/docklet-next/backend/pkg/utils/acfs"
	workspacepkg "github.com/florianfinn/docklet-next/backend/pkg/workspace"
)

// Service reads and edits a project directory as a bounded workspace.
type Service struct {
	config *config.Config
}

func New(cfg *config.Config) *Service {
	return &Service{config: cfg}
}

// Read lists the workspace files.
func (s *Service) Read(ctx context.Context, projectPath, composeFileName string) (*workspacetypes.Workspace, error) {
	files, revision, truncated, err := projects.ReadProjectWorkspace(
		ctx,
		projectPath,
		s.config.ProjectWorkspaceMaxDepth,
		s.config.ProjectScanSkipDirs,
		composeFileName,
		s.config.ProjectWorkspaceMaxEntries,
		workspacepkg.MaxFileSizeBytes(s.config.ProjectWorkspaceMaxFileSizeMB),
	)
	if err != nil {
		return nil, fmt.Errorf("read project workspace: %w", err)
	}
	return &workspacetypes.Workspace{Files: files, FileTreeRevision: revision, FileTreeTruncated: truncated}, nil
}

// File returns one workspace file's content and whether it may be edited.
func (s *Service) File(ctx context.Context, projectPath, composeFileName, relativePath string) (*workspacetypes.FileContent, error) {
	rel, fullPath, entry, err := resolvePath(ctx, projectPath, composeFileName, relativePath)
	if err != nil {
		return nil, err
	}
	response := &workspacetypes.FileContent{
		Path:         fullPath,
		RelativePath: rel,
		Name:         entry.Name,
		Size:         entry.Size,
	}
	if entry.IsDirectory {
		return nil, common.Classify(common.ErrProjectWorkspaceBadRequest, errors.New("workspace path is a directory"))
	}
	if entry.IsSymlink {
		response.ReadOnlyReason = workspacetypes.FileReadOnlySymlink
		return response, nil
	}
	if !acfsutils.IsRegular(entry) {
		response.ReadOnlyReason = workspacetypes.FileReadOnlySpecial
		return response, nil
	}
	maxBytes := workspacepkg.MaxFileSizeBytes(s.config.ProjectWorkspaceMaxFileSizeMB)
	if entry.Size > maxBytes {
		response.ReadOnlyReason = workspacetypes.FileReadOnlyTooLarge
		return response, nil
	}
	reader, _, err := acfs.OpenRead(ctx, projectPath, "/"+rel, maxBytes)
	if err != nil {
		return nil, classifyACFSError(err, "read project workspace file")
	}
	defer func() { _ = reader.Close() }()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read project workspace file: %w", err)
	}
	response.MimeType = http.DetectContentType(content)
	if !workspacepkg.IsTextContent(content) {
		response.ReadOnlyReason = workspacetypes.FileReadOnlyBinary
		return response, nil
	}
	response.Content = string(content)
	response.Editable = true
	return response, nil
}

// Download opens one workspace file for streaming.
func (s *Service) Download(ctx context.Context, projectPath, composeFileName, relativePath string) (io.ReadCloser, int64, string, error) {
	rel, _, entry, err := resolvePath(ctx, projectPath, composeFileName, relativePath)
	if err != nil {
		return nil, 0, "", err
	}
	if entry.IsDirectory {
		return nil, 0, "", common.Classify(common.ErrProjectWorkspaceBadRequest, errors.New("workspace path is a directory"))
	}
	file, size, err := acfs.OpenRead(ctx, projectPath, "/"+rel, 0)
	if err != nil {
		return nil, 0, "", classifyACFSError(err, "open project workspace file")
	}
	return file, size, filepath.Base(rel), nil
}

// Apply backs up the touched paths, applies the manifest and restores the
// backup when the changes cannot be applied.
func (s *Service) Apply(
	ctx context.Context,
	projectsDirectory, projectPath, composeFileName string,
	manifest project.WorkspaceUpdateManifest,
	uploads map[int][]byte,
) error {
	scope := projects.ProjectUpdateBackupScope{}
	for _, change := range manifest.FileChanges {
		scope.Paths = append(scope.Paths, changeTargetPaths(change)...)
	}
	backup, cleanup, err := projects.BackupProjectDirectory(ctx, projectsDirectory, projectPath, ".project-update-backup-*", scope)
	if err != nil {
		return err
	}
	defer cleanup()

	opts := projects.ProjectWorkspaceApplyOptions{
		ExpectedRevision: strings.TrimSpace(manifest.FileTreeRevision),
		MaxDepth:         s.config.ProjectWorkspaceMaxDepth,
		MaxEntries:       s.config.ProjectWorkspaceMaxEntries,
		MaxFileSizeBytes: workspacepkg.MaxFileSizeBytes(s.config.ProjectWorkspaceMaxFileSizeMB),
		SkipDirectories:  s.config.ProjectScanSkipDirs,
		ComposeFileName:  composeFileName,
	}
	if applyErr := projects.ApplyProjectWorkspaceChanges(ctx, projectPath, manifest.FileChanges, uploads, opts); applyErr != nil {
		if restoreErr := projects.RestoreProjectUpdateBackup(ctx, projectPath, backup); restoreErr != nil {
			return errors.Join(WrapProjectWorkspaceError(applyErr), fmt.Errorf("rollback project workspace: %w", restoreErr))
		}
		return WrapProjectWorkspaceError(applyErr)
	}
	return nil
}

// resolvePath validates a workspace path outside protected config and stats it.
func resolvePath(ctx context.Context, projectPath, composeFileName, relativePath string) (string, string, types.Entry, error) {
	rel, err := kit.NormalizeRelativePath(relativePath)
	if err != nil {
		return "", "", types.Entry{}, common.Classify(common.ErrProjectWorkspaceForbidden, fmt.Errorf("invalid project workspace path: %w", err))
	}
	rootName, _, _ := strings.Cut(rel, "/")
	protected := projects.ProtectedProjectPaths(projectPath, composeFileName)
	if protected[rel] || protected[rootName] {
		return "", "", types.Entry{}, common.Classify(common.ErrProjectWorkspaceForbidden, errors.New("project configuration is not part of the workspace"))
	}
	fullPath := filepath.Join(projectPath, filepath.FromSlash(rel))
	entry, err := acfs.Stat(ctx, projectPath, "/"+rel, false)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", types.Entry{}, common.Classify(common.ErrProjectWorkspaceNotFound, errors.New("project workspace file not found"))
		}
		return "", "", types.Entry{}, classifyACFSError(err, "inspect project workspace file")
	}
	return rel, fullPath, entry, nil
}

// WrapProjectWorkspaceError classifies workspace apply failures.
func WrapProjectWorkspaceError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, projects.ErrProjectWorkspaceRevisionConflict):
		return common.Classify(common.ErrProjectWorkspaceConflict, err)
	case errors.Is(err, acfs.ErrAlreadyExists), errors.Is(err, acfs.ErrNotEmpty):
		return common.Classify(common.ErrProjectWorkspaceConflict, fmt.Errorf("conflicting project workspace path: %w", err))
	case errors.Is(err, projects.ErrProjectWorkspaceOutsideProjectDirectory),
		errors.Is(err, projects.ErrProjectWorkspaceProtectedPath),
		errors.Is(err, projects.ErrProjectWorkspaceSymlinkPath),
		errors.Is(err, acfs.ErrOutsideRoot),
		errors.Is(err, acfs.ErrInvalidPath),
		errors.Is(err, acfs.ErrSymlinkLoop),
		errors.Is(err, acfs.ErrSymlink):
		return common.Classify(common.ErrProjectWorkspaceForbidden, fmt.Errorf("forbidden project workspace path: %w", err))
	default:
		return common.Classify(common.ErrProjectWorkspaceBadRequest, fmt.Errorf("invalid project workspace request: %w", err))
	}
}

// changeTargetPaths lists every normalized path a change can create, overwrite or
// delete, including source and destination paths for move operations.
func changeTargetPaths(change project.WorkspaceFileChange) []string {
	rel, err := kit.NormalizeRelativePath(change.RelativePath)
	if err != nil {
		return nil
	}
	paths := []string{rel}
	switch change.Operation {
	case project.FileOpRename:
		if newName, nameErr := kit.ValidateFileName(change.NewName); nameErr == nil {
			paths = append(paths, filepath.ToSlash(filepath.Join(filepath.Dir(rel), newName)))
		}
	case project.FileOpMove:
		paths = append(paths, filepath.ToSlash(filepath.Join(change.NewParentPath, filepath.Base(rel))))
	}
	return paths
}

// classifyACFSError classifies workspace read failures.
func classifyACFSError(err error, operation string) error {
	switch {
	case errors.Is(err, acfs.ErrInvalidPath), errors.Is(err, acfs.ErrOutsideRoot), errors.Is(err, acfs.ErrSymlinkLoop), errors.Is(err, acfs.ErrSymlink):
		return common.Classify(common.ErrProjectWorkspaceForbidden, fmt.Errorf("%s: %w", operation, err))
	case errors.Is(err, acfs.ErrAlreadyExists), errors.Is(err, acfs.ErrNotEmpty):
		return common.Classify(common.ErrProjectWorkspaceConflict, fmt.Errorf("%s: %w", operation, err))
	case os.IsNotExist(err):
		return common.Classify(common.ErrProjectWorkspaceNotFound, errors.New("project workspace file not found"))
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
}
