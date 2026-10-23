// Source: getarcaneapp/arcane/backend/internal/volume/children/workspace/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; mutation and rollback helpers extracted to mutations.go.

package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/florianfinn/docklet-next/types/workspace"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.getarcane.app/acfs"
	"go.getarcane.app/acfs/types"
	"go.getarcane.app/docker"
	"go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	acfsutils "github.com/florianfinn/docklet-next/backend/pkg/utils/acfs"
	workspacepkg "github.com/florianfinn/docklet-next/backend/pkg/workspace"
)

// Dependencies are the helper-container, lock, and backup operations the workspace runs through.
type Dependencies struct {
	Docker                *dockerInternal.DockerClientService
	Events                *event.EventService
	Locks                 *utils.KeyedMutex
	AcquireHelper         func(ctx context.Context, volumeName string) (string, func(), error)
	RequireACFS           func(ctx context.Context, volumeName, containerID string) error
	Exec                  func(ctx context.Context, containerID, execUser string, cmd []string) (string, string, error)
	HelperImage           func(ctx context.Context, dockerClient *client.Client) (string, error)
	BackupStorageMount    func(ctx context.Context, dockerClient *client.Client, target string, readOnly bool) (mount.Mount, error)
	ArchiveFileTarget     func(ctx context.Context, volumeName, backupID, rel string) (string, string, error)
	RestoreArchiveMembers func(ctx context.Context, containerID, filename string, cleanedPaths []string) (string, error)
	MaxDepth              int
	MaxEntries            int
	MaxFileSizeBytes      int64
}

// Service browses and edits the files inside a volume through its helper container.
type Service struct {
	deps Dependencies
}

func NewService(deps Dependencies) *Service {
	return &Service{deps: deps}
}

// MaxFileSizeBytes is the largest file the workspace reads or accepts.
func (s *Service) MaxFileSizeBytes() int64 {
	return s.volumeWorkspaceMaxFileSizeBytesInternal()
}

func (s *Service) GetVolumeWorkspace(ctx context.Context, volumeName string) (*workspace.Workspace, error) {
	defer s.deps.Locks.RLock(volumeName)()

	totalStartedAt := time.Now()
	defer func() {
		slog.DebugContext(ctx, "volume workspace load completed", "volume", volumeName, "totalDuration", time.Since(totalStartedAt))
	}()

	inspectionStartedAt := time.Now()
	if err := s.validateVolumeHelperSupportInternal(ctx, volumeName); err != nil {
		return nil, classifyVolumeWorkspaceHelperSupportErrorInternal(err)
	}
	slog.DebugContext(ctx, "volume workspace inspection completed", "volume", volumeName, "duration", time.Since(inspectionStartedAt))

	helperStartedAt := time.Now()
	containerID, cleanup, err := s.deps.AcquireHelper(ctx, volumeName)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if requireVolumeHelperACFSErr := s.deps.RequireACFS(ctx, volumeName, containerID); requireVolumeHelperACFSErr != nil {
		return nil, requireVolumeHelperACFSErr
	}
	slog.DebugContext(ctx, "volume workspace helper acquired", "volume", volumeName, "containerId", containerID, "duration", time.Since(helperStartedAt))

	scanStartedAt := time.Now()
	tree, err := s.readVolumeWorkspaceFromContainerInternal(ctx, containerID)
	if err != nil {
		return nil, err
	}
	slog.DebugContext(
		ctx,
		"volume workspace tree scan completed",
		"volume",
		volumeName,
		"fileCount",
		len(
			tree.Files,
		),
		"truncated",
		tree.FileTreeTruncated,
		"duration",
		time.Since(
			scanStartedAt,
		),
	)
	return tree, nil
}

func (s *Service) readVolumeWorkspaceFromContainerInternal(ctx context.Context, containerID string) (*workspace.Workspace, error) {
	maxDepth := s.deps.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 50
	}
	maxEntries := s.deps.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 10000
	}

	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	pipeReader, pipeWriter := io.Pipe()
	// io.PipeReader.CloseWithError always returns nil.
	stopClose := context.AfterFunc(workCtx, func() { _ = pipeReader.CloseWithError(workCtx.Err()) })
	defer stopClose()
	var stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		cmd := []string{
			"acfs", "walk", "--root", "/volume", "--path", "/",
			"--max-depth", strconv.Itoa(maxDepth), "--max-entries", strconv.Itoa(maxEntries),
		}
		exitCode, execErr := docker.ExecInContainer(workCtx, dockerClient, containerID, client.ExecCreateOptions{
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          cmd,
		}, pipeWriter, &stderr)
		if execErr == nil && exitCode != 0 {
			execErr = fmt.Errorf("acfs walk exited with code %d", exitCode)
		}
		_ = pipeWriter.CloseWithError(execErr)
		done <- execErr
	}()

	tree, parseErr := decodeVolumeWorkspaceWalkInternal(pipeReader, maxEntries, s.volumeWorkspaceMaxFileSizeBytesInternal())
	if parseErr != nil {
		cancel()
		_ = pipeReader.CloseWithError(parseErr)
	}
	execErr := <-done
	_ = pipeReader.Close()
	if execErr != nil && (parseErr == nil || ctx.Err() != nil || !errors.Is(execErr, context.Canceled)) {
		return nil, classifyVolumeWorkspaceExecErrorInternal(execErr, stderr.String(), "read volume workspace")
	}
	if parseErr != nil {
		return nil, fmt.Errorf("parse volume workspace: %w", parseErr)
	}

	return tree, nil
}

func (s *Service) volumeWorkspaceMaxFileSizeBytesInternal() int64 {
	if s.deps.MaxFileSizeBytes <= 0 {
		return workspacepkg.MaxFileSizeBytes(workspacepkg.DefaultMaxFileSizeMB)
	}
	return s.deps.MaxFileSizeBytes
}

func (s *Service) GetVolumeWorkspaceFile(ctx context.Context, volumeName, relativePath string) (*workspace.FileContent, error) {
	defer s.deps.Locks.RLock(volumeName)()

	if err := s.validateVolumeHelperSupportInternal(ctx, volumeName); err != nil {
		return nil, classifyVolumeWorkspaceHelperSupportErrorInternal(err)
	}
	rel, err := kit.NormalizeRelativePath(relativePath)
	if err != nil {
		return nil, common.Classify(common.ErrVolumeWorkspaceForbidden, fmt.Errorf("invalid volume workspace path: %w", err))
	}
	containerID, cleanup, err := s.deps.AcquireHelper(ctx, volumeName)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if requireVolumeHelperACFSErr := s.deps.RequireACFS(ctx, volumeName, containerID); requireVolumeHelperACFSErr != nil {
		return nil, requireVolumeHelperACFSErr
	}
	stdout, stderr, err := s.deps.Exec(ctx, containerID, "", []string{
		"acfs", "stat", "--root", "/volume", "--path", "/" + rel,
	})
	if err != nil {
		return nil, classifyVolumeWorkspaceExecErrorInternal(err, stderr, "inspect volume workspace file")
	}
	var statResponse types.StatResponse
	if unmarshalErr := json.Unmarshal([]byte(stdout), &statResponse); unmarshalErr != nil {
		return nil, fmt.Errorf("parse volume workspace stat: %w", unmarshalErr)
	}
	if statResponse.Version != types.ProtocolVersion {
		return nil, fmt.Errorf("unsupported acfs protocol %d", statResponse.Version)
	}
	entry := statResponse.Entry
	if entry.IsSymlink {
		return &workspace.FileContent{Path: "/" + rel, RelativePath: rel, Name: path.Base(rel), Size: entry.Size, ReadOnlyReason: workspace.FileReadOnlySymlink}, nil
	}
	if entry.IsDirectory {
		return nil, common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("path is a directory"))
	}
	if !acfsutils.IsRegular(entry) {
		return volumeWorkspaceFileContentResponseInternal(rel, "special", entry.Size, nil, s.volumeWorkspaceMaxFileSizeBytesInternal())
	}
	maxFileSizeBytes := s.volumeWorkspaceMaxFileSizeBytesInternal()
	if entry.Size > maxFileSizeBytes {
		return volumeWorkspaceFileContentResponseInternal(rel, "regular", entry.Size, nil, maxFileSizeBytes)
	}

	previewLimit := maxFileSizeBytes
	if previewLimit < int64(^uint64(0)>>1) {
		previewLimit++
	}
	reader, size, err := s.startVolumeWorkspaceReadInternal(ctx, containerID, rel, previewLimit, func() {})
	if err != nil {
		return nil, fmt.Errorf("read volume workspace file: %w", err)
	}
	content, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read volume workspace file content: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close volume workspace file content: %w", closeErr)
	}
	if size > maxFileSizeBytes {
		return volumeWorkspaceFileContentResponseInternal(rel, "regular", size, nil, maxFileSizeBytes)
	}
	return volumeWorkspaceFileContentResponseInternal(rel, "regular", size, content, maxFileSizeBytes)
}

func (s *Service) DownloadVolumeWorkspaceFile(ctx context.Context, volumeName, relativePath string) (io.ReadCloser, int64, error) {
	unlock := s.deps.Locks.RLock(volumeName)
	if err := s.validateVolumeHelperSupportInternal(ctx, volumeName); err != nil {
		unlock()
		return nil, 0, classifyVolumeWorkspaceHelperSupportErrorInternal(err)
	}
	rel, err := kit.NormalizeRelativePath(relativePath)
	if err != nil {
		unlock()
		return nil, 0, common.Classify(common.ErrVolumeWorkspaceForbidden, fmt.Errorf("invalid volume workspace path: %w", err))
	}
	containerID, cleanup, err := s.deps.AcquireHelper(ctx, volumeName)
	if err != nil {
		unlock()
		return nil, 0, err
	}
	cleanupAndUnlock := func() {
		cleanup()
		unlock()
	}
	if requireVolumeHelperACFSErr := s.deps.RequireACFS(ctx, volumeName, containerID); requireVolumeHelperACFSErr != nil {
		cleanupAndUnlock()
		return nil, 0, requireVolumeHelperACFSErr
	}
	return s.startVolumeWorkspaceReadInternal(ctx, containerID, rel, 0, cleanupAndUnlock)
}

type volumeWorkspaceReadStreamInternal struct {
	ctx       context.Context
	cancel    context.CancelFunc
	stopClose func() bool
	pipe      *io.PipeReader
	done      <-chan error
	cleanup   func()
	remaining int64
	once      sync.Once
	err       error
}

func (r *volumeWorkspaceReadStreamInternal) Read(buffer []byte) (int, error) {
	if r.remaining == 0 {
		var extra [1]byte
		read, readErr := r.pipe.Read(extra[:])
		closeErr := r.Close()
		if read != 0 {
			return 0, errors.New("acfs read emitted bytes beyond its framed payload")
		}
		if closeErr != nil {
			return 0, closeErr
		}
		return 0, kit.Ternary(readErr != nil && !errors.Is(readErr, io.EOF), readErr, io.EOF)
	}
	if int64(len(buffer)) > r.remaining {
		buffer = buffer[:r.remaining]
	}
	read, err := r.pipe.Read(buffer)
	r.remaining -= int64(read)
	if err != nil && r.remaining > 0 {
		closeErr := r.Close()
		if closeErr != nil {
			return read, errors.Join(err, closeErr)
		}
	}
	return read, err
}

func (r *volumeWorkspaceReadStreamInternal) Close() error {
	r.once.Do(func() {
		r.stopClose()
		r.cancel()
		_ = r.pipe.Close()
		r.err = <-r.done
		if r.ctx.Err() == nil && errors.Is(r.err, context.Canceled) {
			r.err = nil
		}
		r.cleanup()
	})
	return r.err
}

func (s *Service) startVolumeWorkspaceReadInternal(ctx context.Context, containerID, relativePath string, limit int64, cleanup func()) (io.ReadCloser, int64, error) {
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		cleanup()
		return nil, 0, err
	}
	workCtx, cancel := context.WithCancel(ctx)
	pipeReader, pipeWriter := io.Pipe()
	// io.PipeReader.CloseWithError always returns nil.
	stopClose := context.AfterFunc(workCtx, func() { _ = pipeReader.CloseWithError(workCtx.Err()) })
	done := make(chan error, 1)
	cmd := []string{"acfs", "read", "--root", "/volume", "--path", "/" + relativePath}
	if limit > 0 {
		cmd = append(cmd, "--limit", strconv.FormatInt(limit, 10))
	}
	go func() {
		var stderr bytes.Buffer
		exitCode, execErr := docker.ExecInContainer(workCtx, dockerClient, containerID, client.ExecCreateOptions{
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          cmd,
		}, pipeWriter, &stderr)
		if execErr == nil && exitCode != 0 {
			execErr = fmt.Errorf("acfs read exited with code %d", exitCode)
		}
		if execErr != nil {
			execErr = classifyVolumeWorkspaceExecErrorInternal(execErr, stderr.String(), "read volume workspace file")
		}
		stopClose()
		_ = pipeWriter.CloseWithError(execErr)
		done <- execErr
	}()

	payloadSize, err := acfs.ReadStreamHeader(pipeReader)
	if err != nil {
		cancel()
		_ = pipeReader.CloseWithError(err)
		execErr := <-done
		cleanup()
		if execErr != nil && (ctx.Err() != nil || !errors.Is(execErr, context.Canceled)) {
			return nil, 0, execErr
		}
		return nil, 0, fmt.Errorf("parse volume workspace read header: %w", err)
	}
	if payloadSize > uint64(1<<63-1) {
		cancel()
		_ = pipeReader.Close()
		<-done
		cleanup()
		return nil, 0, errors.New("volume workspace file is too large")
	}
	size := int64(payloadSize)
	return &volumeWorkspaceReadStreamInternal{
		ctx:       ctx,
		cancel:    cancel,
		stopClose: stopClose,
		pipe:      pipeReader,
		done:      done,
		cleanup:   cleanup,
		remaining: size,
	}, size, nil
}

func (s *Service) validateVolumeHelperSupportInternal(ctx context.Context, volumeName string) error {
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}
	result, err := dockerClient.VolumeInspect(ctx, volumeName, client.VolumeInspectOptions{})
	if err != nil {
		return fmt.Errorf("failed to inspect volume: %w", err)
	}
	if options := result.Volume.Options; options["type"] == "none" || strings.Contains(options["o"], "bind") {
		return fmt.Errorf("volume %q uses a custom mount configuration and cannot be accessed through the workspace helper", volumeName)
	}
	return nil
}

func (s *Service) validateVolumeWorkspacePathInternal(ctx context.Context, containerID, relativePath string, allowMissing bool) error {
	allowMissingValue := kit.Ternary(allowMissing, "1", "0")
	_, stderr, err := s.deps.Exec(ctx, containerID, "", []string{"sh", "-c", volumeWorkspaceValidatePathScriptInternal, "sh", relativePath, allowMissingValue})
	if err != nil {
		return classifyVolumeWorkspaceExecErrorInternal(err, stderr, "validate volume workspace path")
	}
	return nil
}

const (
	volumeWorkspaceValidatePathScriptInternal = `set -e
rel="$1"
allow_missing="$2"
cur=/volume
remaining=$rel
while :; do
  case "$remaining" in '') break ;; esac
  case "$remaining" in
    */*) segment=${remaining%%/*}; remaining=${remaining#*/} ;;
    *) segment=$remaining; remaining= ;;
  esac
  cur="$cur/$segment"
  if mode=$(stat -c '%A' -- "$cur" 2>/dev/null); then
    case "$mode" in l*) echo ARCANE_SYMLINK >&2; exit 42 ;; esac
  else
    case "$allow_missing" in 1) exit 0 ;; esac
    echo ARCANE_NOT_FOUND >&2
    exit 44
  fi
  case "$remaining" in
    '') ;;
    *) case "$mode" in d*) ;; *) echo ARCANE_NOT_DIRECTORY >&2; exit 47 ;; esac ;;
  esac
done`

	volumeWorkspaceBackupCreateScriptInternal = `set -e
rel="$1"
archive="$2"
cur=/volume
current=
remaining=$rel
while :; do
  case "$remaining" in '') break ;; esac
  case "$remaining" in
    */*) segment=${remaining%%/*}; remaining=${remaining#*/} ;;
    *) segment=$remaining; remaining= ;;
  esac
  case "$current" in '') current=$segment ;; *) current="$current/$segment" ;; esac
  cur="$cur/$segment"
  if mode=$(stat -c '%A' -- "$cur" 2>/dev/null); then
    case "$mode" in l*) echo ARCANE_SYMLINK >&2; exit 42 ;; esac
  else
    printf 'absent\0%s\0' "$current"
    exit 0
  fi
  case "$remaining" in
    '') ;;
    *) case "$mode" in d*) ;; *) echo ARCANE_NOT_DIRECTORY >&2; exit 47 ;; esac ;;
  esac
done
case "$rel" in
  */*) parent="/volume/${rel%/*}"; entry=${rel##*/} ;;
  *) parent=/volume; entry=$rel ;;
esac
cd "$parent"
tar -cf "$archive" "./$entry"
printf 'present\0'`
)

func (
	s *Service,
) UpdateVolumeWorkspace(
	ctx context.Context,
	volumeName string,
	manifest volume.WorkspaceUpdateManifest,
	uploads map[int][]byte,
	user usertypes.Actor,
) (
	*workspace.Workspace,
	error,
) {
	totalStartedAt := time.Now()
	defer func() {
		slog.DebugContext(ctx, "volume workspace update completed", "volume", volumeName, "fileChangeCount", len(manifest.FileChanges), "totalDuration", time.Since(totalStartedAt))
	}()

	if err := workspacepkg.ValidateUpdateManifest(manifest.FileTreeRevision, len(manifest.FileChanges), 500); err != nil {
		return nil, common.Classify(common.ErrVolumeWorkspaceBadRequest, err)
	}
	uploadReferences := make([]workspacepkg.UploadReference, 0, len(manifest.FileChanges))
	for _, change := range manifest.FileChanges {
		uploadReferences = append(uploadReferences, workspacepkg.UploadReference{Operation: change.Operation, UploadIndex: change.UploadIndex, BaselineIndex: change.BaselineIndex})
		if err := validateVolumeWorkspaceFileChangeInternal(change); err != nil {
			return nil, common.Classify(common.ErrVolumeWorkspaceBadRequest, err)
		}
	}
	if err := workspacepkg.ValidateUploadIndices(uploadReferences, len(uploads), volume.FileOpCreateFile, volume.FileOpUpdateFile); err != nil {
		return nil, common.Classify(common.ErrVolumeWorkspaceBadRequest, err)
	}
	inspectionStartedAt := time.Now()
	if err := s.validateVolumeHelperSupportInternal(ctx, volumeName); err != nil {
		return nil, classifyVolumeWorkspaceHelperSupportErrorInternal(err)
	}
	slog.DebugContext(ctx, "volume workspace inspection completed", "volume", volumeName, "duration", time.Since(inspectionStartedAt))
	defer s.deps.Locks.Lock(volumeName)()

	needsBackups := slices.ContainsFunc(manifest.FileChanges, func(change volume.WorkspaceFileChange) bool {
		return change.Operation == volume.FileOpRestoreFile
	})
	helperStartedAt := time.Now()
	containerID, cleanup, err := s.createVolumeWorkspaceMutationContainerInternal(ctx, volumeName, needsBackups)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if requireVolumeHelperACFSErr := s.deps.RequireACFS(ctx, volumeName, containerID); requireVolumeHelperACFSErr != nil {
		return nil, requireVolumeHelperACFSErr
	}
	slog.DebugContext(ctx, "volume workspace mutation helper acquired", "volume", volumeName, "containerId", containerID, "dedicated", needsBackups, "duration", time.Since(helperStartedAt))

	revisionScanStartedAt := time.Now()
	current, err := s.readVolumeWorkspaceFromContainerInternal(ctx, containerID)
	if err != nil {
		return nil, err
	}
	slog.DebugContext(
		ctx,
		"volume workspace revision tree scan completed",
		"volume",
		volumeName,
		"fileCount",
		len(
			current.Files,
		),
		"truncated",
		current.FileTreeTruncated,
		"duration",
		time.Since(
			revisionScanStartedAt,
		),
	)
	if validateVolumeWorkspaceRevisionErr := validateVolumeWorkspaceRevisionInternal(manifest.FileTreeRevision, current.FileTreeRevision); validateVolumeWorkspaceRevisionErr != nil {
		return nil, validateVolumeWorkspaceRevisionErr
	}

	stagingStartedAt := time.Now()
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	identity := s.resolveVolumeWorkspaceWriteIdentityInternal(ctx, dockerClient, containerID, volumeName)
	stagedFiles, err := s.stageVolumeWorkspaceChangesInternal(ctx, dockerClient, containerID, manifest.FileChanges, uploads, identity)
	if err != nil {
		return nil, err
	}
	slog.DebugContext(ctx, "volume workspace staging completed", "volume", volumeName, "stagedFileCount", len(stagedFiles), "duration", time.Since(stagingStartedAt))

	backupStartedAt := time.Now()
	scope, err := volumeWorkspaceBackupScopeInternal(manifest.FileChanges)
	if err != nil {
		return nil, common.Classify(common.ErrVolumeWorkspaceBadRequest, err)
	}
	for _, relativePath := range scope {
		if validateVolumeWorkspacePathErr := s.validateVolumeWorkspacePathInternal(ctx, containerID, relativePath, true); validateVolumeWorkspacePathErr != nil {
			return nil, validateVolumeWorkspacePathErr
		}
	}
	backup, err := s.backupVolumeWorkspaceScopeInternal(ctx, containerID, scope)
	if err != nil {
		return nil, err
	}
	slog.DebugContext(ctx, "volume workspace backup completed", "volume", volumeName, "scopeCount", len(scope), "duration", time.Since(backupStartedAt))

	applyStartedAt := time.Now()
	if applyVolumeWorkspaceChangesErr := s.applyVolumeWorkspaceChangesInternal(
		ctx,
		dockerClient,
		containerID,
		volumeName,
		manifest.FileChanges,
		stagedFiles,
		identity,
		func() error { return s.restoreVolumeWorkspaceScopeInternal(ctx, containerID, backup) },
	); applyVolumeWorkspaceChangesErr != nil {
		return nil, applyVolumeWorkspaceChangesErr
	}
	slog.DebugContext(ctx, "volume workspace changes applied", "volume", volumeName, "fileChangeCount", len(manifest.FileChanges), "duration", time.Since(applyStartedAt))

	finalScanStartedAt := time.Now()
	tree, err := s.readVolumeWorkspaceFromContainerInternal(ctx, containerID)
	if err != nil {
		return nil, err
	}
	slog.DebugContext(
		ctx,
		"volume workspace final tree scan completed",
		"volume",
		volumeName,
		"fileCount",
		len(
			tree.Files,
		),
		"truncated",
		tree.FileTreeTruncated,
		"duration",
		time.Since(
			finalScanStartedAt,
		),
	)

	if s.deps.Events != nil {
		metadata := database.JSON{"action": "workspace_update", "fileChangeCount": len(manifest.FileChanges)}
		if logErr := s.deps.Events.LogVolumeEvent(ctx, event.EventTypeVolumeWorkspaceUpdate, volumeName, volumeName, user.ID, user.Username, "0", metadata); logErr != nil {
			slog.WarnContext(ctx, "could not log volume workspace update event", "volume", volumeName, "error", logErr.Error())
		}
	}
	return tree, nil
}

func validateVolumeWorkspaceFileChangeInternal(change volume.WorkspaceFileChange) error {
	if _, err := kit.NormalizeRelativePath(change.RelativePath); err != nil {
		return fmt.Errorf("invalid volume workspace path: %w", err)
	}
	hasUpload := change.UploadIndex != nil
	switch change.Operation {
	case volume.FileOpCreateFile, volume.FileOpUpdateFile:
		if !hasUpload {
			return errors.New("create_file and update_file require uploadIndex")
		}
	case volume.FileOpCreateFolder, volume.FileOpDelete:
		if hasUpload {
			return errors.New("operation does not accept file content")
		}
	case volume.FileOpRename:
		if hasUpload {
			return errors.New("rename does not accept file content")
		}
		if _, err := kit.ValidateFileName(change.NewName); err != nil {
			return fmt.Errorf("invalid volume workspace file name: %w", err)
		}
	case volume.FileOpMove:
		if hasUpload {
			return errors.New("move does not accept file content")
		}
		if strings.TrimSpace(change.NewParentPath) != "" {
			if _, err := kit.NormalizeRelativePath(change.NewParentPath); err != nil {
				return fmt.Errorf("invalid destination folder: %w", err)
			}
		}
	case volume.FileOpRestoreFile:
		if hasUpload {
			return errors.New("restore_file does not accept file content")
		}
		if strings.TrimSpace(change.BackupID) == "" {
			return errors.New("restore_file requires backupId")
		}
	default:
		return fmt.Errorf("unsupported volume workspace operation %q", change.Operation)
	}
	return nil
}

func validateVolumeWorkspaceRevisionInternal(expected, current string) error {
	if strings.TrimSpace(expected) != current {
		return common.Classify(common.ErrVolumeWorkspaceConflict, errors.New("volume workspace changed; refresh it and try again"))
	}
	return nil
}

func classifyVolumeWorkspaceHelperSupportErrorInternal(err error) error {
	switch {
	case errdefs.IsNotFound(err):
		return common.Classify(common.ErrVolumeWorkspaceNotFound, err)
	case strings.Contains(err.Error(), "uses a custom mount configuration"):
		return common.Classify(common.ErrVolumeWorkspaceBadRequest, err)
	default:
		return err
	}
}

func classifyVolumeWorkspaceExecErrorInternal(err error, stderr, fallbackContext string) error {
	var response types.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &response); decodeErr == nil && response.Version == types.ProtocolVersion {
		switch response.Code {
		case types.ErrorInvalidPath, types.ErrorOutsideRoot, types.ErrorSymlink, types.ErrorSymlinkLoop:
			return common.Classify(common.ErrVolumeWorkspaceForbidden, errors.New(response.Message))
		case types.ErrorNotFound:
			return common.Classify(common.ErrVolumeWorkspaceNotFound, errors.New("volume workspace file not found"))
		case types.ErrorAlreadyExist:
			return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("volume workspace destination already exists"))
		case types.ErrorNotDirectory:
			return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("volume workspace parent is not a directory"))
		case types.ErrorNotFile:
			return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("volume workspace path is not a regular file"))
		case types.ErrorNotEmpty:
			return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("volume directory is not empty; recursive delete is required"))
		case types.ErrorIsDirectory:
			return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("path is a directory"))
		case types.ErrorSizeMismatch, types.ErrorRootRemoval:
			return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New(response.Message))
		case types.ErrorInternal:
			return fmt.Errorf("%s: %w", fallbackContext, err)
		}
	}

	message := stderr + " " + err.Error()
	switch {
	case strings.Contains(message, "ARCANE_SYMLINK"):
		return common.Classify(common.ErrVolumeWorkspaceForbidden, errors.New("symlink volume paths are not supported"))
	case strings.Contains(message, "ARCANE_NOT_FOUND"):
		return common.Classify(common.ErrVolumeWorkspaceNotFound, errors.New("volume workspace file not found"))
	case strings.Contains(message, "ARCANE_COLLISION"):
		return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("volume workspace destination already exists"))
	case strings.Contains(message, "ARCANE_NOT_DIRECTORY"):
		return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("volume workspace parent is not a directory"))
	case strings.Contains(message, "ARCANE_NOT_FILE"):
		return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("volume workspace path is not a regular file"))
	case strings.Contains(message, "ARCANE_NOT_EMPTY"):
		return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("volume directory is not empty; recursive delete is required"))
	case strings.Contains(message, "ARCANE_DIRECTORY"):
		return common.Classify(common.ErrVolumeWorkspaceBadRequest, errors.New("path is a directory"))
	default:
		return fmt.Errorf("%s: %w", fallbackContext, err)
	}
}

func volumeWorkspaceFileContentResponseInternal(relativePath, kind string, size int64, content []byte, maxFileSizeBytes int64) (*workspace.FileContent, error) {
	response := &workspace.FileContent{
		Path:         "/" + relativePath,
		RelativePath: relativePath,
		Name:         path.Base(relativePath),
		MimeType:     http.DetectContentType(content),
		Size:         size,
	}
	if kind == "special" {
		response.ReadOnlyReason = workspace.FileReadOnlySpecial
		return response, nil
	}
	if kind != "regular" {
		return nil, errors.New("invalid volume workspace file kind")
	}
	if size > maxFileSizeBytes {
		response.ReadOnlyReason = workspace.FileReadOnlyTooLarge
		return response, nil
	}
	if !workspacepkg.IsTextContent(content) {
		response.ReadOnlyReason = workspace.FileReadOnlyBinary
		return response, nil
	}
	response.Editable = true
	response.Content = string(content)
	return response, nil
}

func volumeWorkspaceEntryFromACFSInternal(entry types.Entry, maxFileSizeBytes int64) (workspace.FileEntry, string) {
	fileEntry := acfsutils.FileEntry(entry)
	fileEntry.RelativePath = strings.TrimPrefix(entry.Path, "/")
	switch {
	case entry.IsDirectory:
		fileEntry.Size = 0
		fileEntry.Editable = true
		return fileEntry, "dir"
	case entry.IsSymlink:
		fileEntry.ReadOnlyReason = workspace.FileReadOnlySymlink
		return fileEntry, "symlink"
	case acfsutils.IsRegular(entry):
		if entry.Size > maxFileSizeBytes {
			fileEntry.ReadOnlyReason = workspace.FileReadOnlyTooLarge
		} else {
			fileEntry.Editable = true
		}
		return fileEntry, "file"
	default:
		fileEntry.ReadOnlyReason = workspace.FileReadOnlySpecial
		return fileEntry, "special"
	}
}

func decodeVolumeWorkspaceWalkInternal(source io.Reader, maxEntries int, maxFileSizeBytes int64) (*workspace.Workspace, error) {
	decoder := jsontext.NewDecoder(source)
	files := make([]workspace.FileEntry, 0, min(maxEntries, 256))
	classifications := make(map[string]string, min(maxEntries, 256))
	trailerSeen := false
	truncated := false
	for {
		var record types.WalkRecord
		err := json.UnmarshalDecode(decoder, &record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if record.Version != types.ProtocolVersion {
			return nil, fmt.Errorf("unsupported acfs protocol %d", record.Version)
		}
		if trailerSeen {
			return nil, errors.New("acfs walk emitted a record after its trailer")
		}
		if record.End {
			trailerSeen = true
			truncated = record.Truncated
			if record.Count != len(files) {
				return nil, errors.New("acfs walk trailer count does not match emitted entries")
			}
			continue
		}
		if record.Entry == nil {
			return nil, errors.New("acfs walk emitted an empty record")
		}
		if len(files) >= maxEntries {
			return nil, errors.New("acfs walk exceeded the requested entry limit")
		}
		fileEntry, classification := volumeWorkspaceEntryFromACFSInternal(*record.Entry, maxFileSizeBytes)
		classifications[fileEntry.RelativePath] = classification
		files = append(files, fileEntry)
	}
	if !trailerSeen {
		return nil, errors.New("acfs walk ended without a trailer")
	}

	revisionEntries := slices.Clone(files)
	slices.SortFunc(revisionEntries, func(a, b workspace.FileEntry) int {
		return strings.Compare(a.RelativePath, b.RelativePath)
	})
	h := sha256.New()
	for _, entry := range revisionEntries {
		kit.WriteRecord(h, entry.RelativePath, classifications[entry.RelativePath], entry.Size, entry.ModTime.UnixNano(), entry.Mode)
	}
	slices.SortFunc(files, func(a, b workspace.FileEntry) int {
		if a.IsDirectory != b.IsDirectory {
			return kit.Ternary(a.IsDirectory, -1, 1)
		}
		return strings.Compare(a.RelativePath, b.RelativePath)
	})
	return &workspace.Workspace{
		Files:             files,
		FileTreeRevision:  hex.EncodeToString(h.Sum(nil)),
		FileTreeTruncated: truncated,
	}, nil
}
