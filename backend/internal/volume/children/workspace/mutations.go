// Source: getarcaneapp/arcane/backend/internal/volume/children/workspace/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extracted mutation/rollback helpers; compare optional update baselines at ordered apply boundaries.

package workspace

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.getarcane.app/acfs"
	"go.getarcane.app/acfs/types"
	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"
	"go.getarcane.app/kit/pkg"
	"io"
	"log/slog"
	"path"
	"slices"
	"strconv"
	"strings"
)

// volumeWorkspaceWriteIdentityInternal is the identity workspace file mutations run as,
// so files land owned by the user the consuming container runs with. The zero value means root.
type volumeWorkspaceWriteIdentityInternal struct {
	execUser string
	uid      int
	gid      int
}

func volumeWorkspaceWriteIdentityFromConfigUserInternal(configUser string) volumeWorkspaceWriteIdentityInternal {
	trimmed := strings.TrimSpace(configUser)
	if trimmed == "" {
		return volumeWorkspaceWriteIdentityInternal{}
	}
	// Named users cannot be resolved against the helper image, and uid 0 is already the default.
	uidPart, gidPart, hasGID := strings.Cut(trimmed, ":")
	uid, err := strconv.ParseUint(uidPart, 10, 32)
	if err != nil || uid == 0 {
		return volumeWorkspaceWriteIdentityInternal{}
	}
	var gid uint64
	if hasGID {
		gid, err = strconv.ParseUint(gidPart, 10, 32)
		if err != nil {
			return volumeWorkspaceWriteIdentityInternal{}
		}
	}
	return volumeWorkspaceWriteIdentityInternal{execUser: trimmed, uid: int(uid), gid: int(gid)}
}

func (s *Service) resolveVolumeWorkspaceWriteIdentityInternal(ctx context.Context, dockerClient *client.Client, containerID, volumeName string) volumeWorkspaceWriteIdentityInternal {
	consumerIDs, err := docker.GetContainersUsingVolume(ctx, dockerClient, volumeName)
	if err != nil {
		slog.WarnContext(ctx, "could not list containers for volume workspace write identity", "volume", volumeName, "error", err.Error())
	}
	identity := volumeWorkspaceWriteIdentityInternal{}
	identitySource := ""
	for _, consumerID := range consumerIDs {
		inspect, inspectErr := compat.ContainerInspectWithCompatibility(ctx, dockerClient, consumerID, client.ContainerInspectOptions{})
		if inspectErr != nil || inspect.Container.Config == nil {
			// An uninspectable consumer could declare a conflicting user, so the identity is
			// indeterminate; defer to the volume root's owner.
			slog.WarnContext(ctx, "could not inspect volume consumer; using volume root owner for workspace writes", "volume", volumeName, "containerId", consumerID)
			identity = volumeWorkspaceWriteIdentityInternal{}
			break
		}
		parsed := volumeWorkspaceWriteIdentityFromConfigUserInternal(inspect.Container.Config.User)
		if parsed.execUser == "" {
			continue
		}
		if identity.execUser == "" {
			identity, identitySource = parsed, consumerID
			continue
		}
		// Consumers disagree; picking either would depend on Docker's list order, so defer to
		// the volume root's owner instead.
		if identity.execUser != parsed.execUser {
			slog.WarnContext(
				ctx,
				"volume consumers declare conflicting users; using volume root owner for workspace writes",
				"volume",
				volumeName,
				"identity",
				identity.execUser,
				"conflictingIdentity",
				parsed.execUser,
			)
			identity = volumeWorkspaceWriteIdentityInternal{}
			break
		}
	}
	if identity.execUser != "" {
		slog.InfoContext(ctx, "volume workspace writes run as container user", "volume", volumeName, "identity", identity.execUser, "sourceContainerId", identitySource)
		return identity
	}
	// PUID-style images drop privileges after start, so Config.User stays empty; the owner of the
	// volume root is the next best signal for the runtime identity.
	stdout, _, err := s.deps.Exec(ctx, containerID, "", []string{"stat", "-c", "%u:%g", "/volume"})
	if err != nil {
		return volumeWorkspaceWriteIdentityInternal{}
	}
	identity = volumeWorkspaceWriteIdentityFromConfigUserInternal(stdout)
	if identity.execUser != "" {
		slog.InfoContext(ctx, "volume workspace writes run as volume root owner", "volume", volumeName, "identity", identity.execUser)
	}
	return identity
}

type volumeWorkspaceStagedFileInternal struct {
	path        string
	size        int64
	baseline    []byte
	baselineSet bool
}

type volumeWorkspaceStagedContentInternal struct {
	name    string
	content io.ReadCloser
	size    int64
}

func (
	s *Service,
) stageVolumeWorkspaceChangesInternal(
	ctx context.Context,
	dockerClient *client.Client,
	containerID string,
	changes []volume.WorkspaceFileChange,
	uploads map[int][]byte,
	identity volumeWorkspaceWriteIdentityInternal,
) (
	map[int]volumeWorkspaceStagedFileInternal,
	error,
) {
	if _, _, err := s.deps.Exec(ctx, containerID, "", []string{"sh", "-c", "rm -rf -- /tmp/arcane-workspace && mkdir -p -- /tmp/arcane-workspace"}); err != nil {
		return nil, fmt.Errorf("prepare volume workspace staging directory: %w", err)
	}

	stagedFiles := make(map[int]volumeWorkspaceStagedFileInternal)
	stagedContents := make([]volumeWorkspaceStagedContentInternal, 0, len(changes))
	for index, change := range changes {
		if change.UploadIndex == nil {
			continue
		}
		stagedPath := fmt.Sprintf("/tmp/arcane-workspace/change-%d", index)
		content := uploads[*change.UploadIndex]
		reader := io.NopCloser(bytes.NewReader(content))
		size := int64(len(content))
		stagedFiles[index] = volumeWorkspaceStagedFileInternal{path: stagedPath, size: size, baselineSet: change.BaselineIndex != nil}
		if change.BaselineIndex != nil {
			staged := stagedFiles[index]
			staged.baseline = uploads[*change.BaselineIndex]
			stagedFiles[index] = staged
		}
		stagedContents = append(stagedContents, volumeWorkspaceStagedContentInternal{
			name:    path.Base(stagedPath),
			content: reader,
			size:    size,
		})
	}
	if len(stagedContents) == 0 {
		return stagedFiles, nil
	}
	if err := s.copyVolumeWorkspaceFilesToContainerInternal(ctx, dockerClient, containerID, stagedContents, identity); err != nil {
		return nil, err
	}
	return stagedFiles, nil
}

func (s *Service) applyVolumeWorkspaceChangesInternal(
	ctx context.Context,
	dockerClient *client.Client,
	containerID string,
	volumeName string,
	changes []volume.WorkspaceFileChange,
	stagedFiles map[int]volumeWorkspaceStagedFileInternal,
	identity volumeWorkspaceWriteIdentityInternal,
	rollback func() error,
) error {
	rollbackFailureInternal := func(applyErr error) error {
		if rollbackErr := rollback(); rollbackErr != nil {
			return errors.Join(applyErr, fmt.Errorf("failed to roll back volume workspace: %w", rollbackErr))
		}
		return applyErr
	}

	for index := 0; index < len(changes); {
		if changes[index].Operation == volume.FileOpRestoreFile {
			rel, _ := kit.NormalizeRelativePath(changes[index].RelativePath)
			if err := s.restoreVolumeWorkspaceFileInternal(ctx, containerID, volumeName, changes[index].BackupID, rel); err != nil {
				return rollbackFailureInternal(err)
			}
			index++
			continue
		}

		// Compare at the ordered update boundary, after any preceding rename,
		// move or create has been applied. Any failure rolls back the manifest.
		if staged := stagedFiles[index]; changes[index].Operation == volume.FileOpUpdateFile && staged.baselineSet {
			if err := s.checkVolumeWorkspaceBaselineInternal(ctx, containerID, changes[index].RelativePath, staged.baseline); err != nil {
				return rollbackFailureInternal(err)
			}
		}
		end := index + 1
		for end < len(changes) && changes[end].Operation != volume.FileOpRestoreFile && !(changes[end].Operation == volume.FileOpUpdateFile && stagedFiles[end].baselineSet) {
			end++
		}
		if err := s.executeVolumeWorkspaceACFSBatchInternal(ctx, dockerClient, containerID, changes[index:end], stagedFiles, index, identity); err != nil {
			return rollbackFailureInternal(err)
		}
		index = end
	}
	return nil
}

func (s *Service) executeVolumeWorkspaceACFSBatchInternal(
	ctx context.Context,
	dockerClient *client.Client,
	containerID string,
	changes []volume.WorkspaceFileChange,
	stagedFiles map[int]volumeWorkspaceStagedFileInternal,
	startIndex int,
	identity volumeWorkspaceWriteIdentityInternal,
) error {
	applyChanges := make([]types.ApplyChange, 0, len(changes))
	for offset, change := range changes {
		applyChange, err := mapVolumeWorkspaceChangeToACFSInternal(change, stagedFiles[startIndex+offset])
		if err != nil {
			return common.Classify(common.ErrVolumeWorkspaceBadRequest, err)
		}
		applyChanges = append(applyChanges, applyChange)
	}

	manifestContent, err := json.Marshal(types.ApplyManifest{
		Version: types.ProtocolVersion,
		Changes: applyChanges,
	})
	if err != nil {
		return fmt.Errorf("encode volume workspace apply manifest: %w", err)
	}
	manifestName := fmt.Sprintf("manifest-%d.json", startIndex)
	if copyVolumeWorkspaceFilesToContainerErr := s.copyVolumeWorkspaceFilesToContainerInternal(ctx, dockerClient, containerID, []volumeWorkspaceStagedContentInternal{{
		name:    manifestName,
		content: io.NopCloser(bytes.NewReader(manifestContent)),
		size:    int64(len(manifestContent)),
	}}, identity); copyVolumeWorkspaceFilesToContainerErr != nil {
		return copyVolumeWorkspaceFilesToContainerErr
	}

	stdout, stderr, err := s.deps.Exec(ctx, containerID, identity.execUser, []string{
		"acfs", "apply", "--root", "/volume", "--staging", "/tmp/arcane-workspace", "--manifest", manifestName,
	})
	if err != nil {
		return classifyVolumeWorkspaceExecErrorInternal(err, stderr, "apply volume workspace changes")
	}
	var response types.ApplyResponse
	if unmarshalErr := json.Unmarshal([]byte(stdout), &response); unmarshalErr != nil {
		return fmt.Errorf("parse volume workspace apply response: %w", unmarshalErr)
	}
	if response.Version != types.ProtocolVersion || response.Applied != len(applyChanges) {
		return errors.New("invalid volume workspace apply response")
	}
	return nil
}

func mapVolumeWorkspaceChangeToACFSInternal(change volume.WorkspaceFileChange, staged volumeWorkspaceStagedFileInternal) (types.ApplyChange, error) {
	relativePath, err := kit.NormalizeRelativePath(change.RelativePath)
	if err != nil {
		return types.ApplyChange{}, err
	}
	result := types.ApplyChange{Path: "/" + relativePath, Recursive: change.Recursive}
	switch change.Operation {
	case volume.FileOpCreateFile:
		result.Operation = types.ApplyCreateFile
		result.StagedName = path.Base(staged.path)
		result.Size = staged.size
	case volume.FileOpUpdateFile:
		result.Operation = types.ApplyUpdateFile
		result.StagedName = path.Base(staged.path)
		result.Size = staged.size
	case volume.FileOpCreateFolder:
		result.Operation = types.ApplyCreateFolder
		result.Recursive = false
	case volume.FileOpRename:
		newName, nameErr := kit.ValidateFileName(change.NewName)
		if nameErr != nil {
			return types.ApplyChange{}, nameErr
		}
		result.Operation = types.ApplyRename
		result.TargetPath = "/" + path.Join(path.Dir(relativePath), newName)
		result.Recursive = false
	case volume.FileOpMove:
		parent := ""
		if strings.TrimSpace(change.NewParentPath) != "" {
			parent, err = kit.NormalizeRelativePath(change.NewParentPath)
			if err != nil {
				return types.ApplyChange{}, err
			}
		}
		result.Operation = types.ApplyMove
		result.TargetPath = "/" + path.Join(parent, path.Base(relativePath))
		result.Recursive = false
	case volume.FileOpDelete:
		result.Operation = types.ApplyDelete
	default:
		return types.ApplyChange{}, fmt.Errorf("unsupported volume workspace operation %q", change.Operation)
	}
	return result, nil
}

func (
	s *Service,
) copyVolumeWorkspaceFilesToContainerInternal(
	ctx context.Context,
	dockerClient *client.Client,
	containerID string,
	contents []volumeWorkspaceStagedContentInternal,
	identity volumeWorkspaceWriteIdentityInternal,
) error {
	pipeReader, pipeWriter := io.Pipe()
	archiveDone := make(chan error, 1)
	go func() {
		tarWriter := tar.NewWriter(pipeWriter)
		var err error
		for _, stagedContent := range contents {
			if err = tarWriter.WriteHeader(&tar.Header{Name: stagedContent.name, Mode: 0o600, Size: stagedContent.size, Uid: identity.uid, Gid: identity.gid}); err != nil {
				break
			}
			if _, err = io.CopyN(tarWriter, stagedContent.content, stagedContent.size); err != nil {
				break
			}
		}
		if closeErr := tarWriter.Close(); err == nil {
			err = closeErr
		}
		_ = pipeWriter.CloseWithError(err)
		archiveDone <- err
	}()
	_, copyErr := dockerClient.CopyToContainer(ctx, containerID, client.CopyToContainerOptions{DestinationPath: "/tmp/arcane-workspace", Content: pipeReader, CopyUIDGID: identity.execUser != ""})
	_ = pipeReader.Close()
	archiveErr := <-archiveDone
	for _, stagedContent := range contents {
		_ = stagedContent.content.Close()
	}
	if copyErr != nil {
		return fmt.Errorf("stage volume workspace files: %w", copyErr)
	}
	if archiveErr != nil {
		return fmt.Errorf("create volume workspace staging archive: %w", archiveErr)
	}
	return nil
}

func (s *Service) createVolumeWorkspaceMutationContainerInternal(ctx context.Context, volumeName string, withBackups bool) (string, func(), error) {
	if !withBackups {
		return s.deps.AcquireHelper(ctx, volumeName)
	}
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return "", nil, err
	}
	helperImage, err := s.deps.HelperImage(ctx, dockerClient)
	if err != nil {
		return "", nil, err
	}
	backupMount, err := s.deps.BackupStorageMount(ctx, dockerClient, "/backups", true)
	if err != nil {
		return "", nil, err
	}
	config := &container.Config{Image: helperImage, Cmd: []string{"sleep", "infinity"}, NetworkDisabled: true, Labels: volumehelper.Labels()}
	hostConfig := volumehelper.HostConfig(helperImage, []string{volumeName + ":/volume"}, []mount.Mount{backupMount})
	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: config, HostConfig: hostConfig})
	if err != nil {
		return "", nil, fmt.Errorf("create volume workspace helper: %w", err)
	}
	if _, containerStartErr := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); containerStartErr != nil {
		startErr := fmt.Errorf("start volume workspace helper: %w", containerStartErr)
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeouts.DefaultDockerAPI)
		defer cancel()
		if _, cleanupErr := dockerClient.ContainerRemove(cleanupCtx, resp.ID, volumehelper.RemoveOptions()); cleanupErr != nil && !errdefs.IsNotFound(cleanupErr) {
			return "", nil, errors.Join(startErr, fmt.Errorf("remove volume workspace helper after start failure: %w", cleanupErr))
		}
		return "", nil, startErr
	}
	return resp.ID, func() {
		_, _ = dockerClient.ContainerRemove(context.WithoutCancel(ctx), resp.ID, volumehelper.RemoveOptions())
	}, nil
}

type volumeWorkspaceBackupArchiveInternal struct {
	relativePath string
	archivePath  string
}

type volumeWorkspaceBackupInternal struct {
	archives      []volumeWorkspaceBackupArchiveInternal
	absentEntries []string
}

func (s *Service) backupVolumeWorkspaceScopeInternal(ctx context.Context, containerID string, scope []string) (*volumeWorkspaceBackupInternal, error) {
	if _, _, err := s.deps.Exec(ctx, containerID, "", []string{"mkdir", "-p", "/tmp/arcane-workspace"}); err != nil {
		return nil, fmt.Errorf("prepare volume workspace backup directory: %w", err)
	}
	backup := &volumeWorkspaceBackupInternal{
		archives:      make([]volumeWorkspaceBackupArchiveInternal, 0, len(scope)),
		absentEntries: make([]string, 0, len(scope)),
	}
	for index, relativePath := range scope {
		archivePath := fmt.Sprintf("/tmp/arcane-workspace/backup-%d.tar", index)
		stdout, stderr, err := s.deps.Exec(ctx, containerID, "", []string{"sh", "-c", volumeWorkspaceBackupCreateScriptInternal, "sh", relativePath, archivePath})
		if err != nil {
			return nil, classifyVolumeWorkspaceExecErrorInternal(err, stderr, "back up volume workspace path")
		}
		fields := strings.Split(stdout, "\x00")
		switch {
		case len(fields) >= 1 && fields[0] == "present":
			backup.archives = append(backup.archives, volumeWorkspaceBackupArchiveInternal{
				relativePath: relativePath,
				archivePath:  archivePath,
			})
		case len(fields) >= 2 && fields[0] == "absent" && fields[1] != "":
			backup.absentEntries = append(backup.absentEntries, fields[1])
		default:
			return nil, errors.New("invalid volume workspace backup path response")
		}
	}
	backup.absentEntries = normalizeVolumeWorkspaceScopeInternal(backup.absentEntries)
	return backup, nil
}

func volumeWorkspaceRollbackPathsInternal(backup *volumeWorkspaceBackupInternal) []string {
	removePaths := slices.Clone(backup.absentEntries)
	for _, archive := range backup.archives {
		removePaths = append(removePaths, archive.relativePath)
	}
	slices.SortFunc(removePaths, func(a, b string) int {
		depthA := strings.Count(a, "/")
		depthB := strings.Count(b, "/")
		if depthA != depthB {
			return depthB - depthA
		}
		return strings.Compare(b, a)
	})
	return removePaths
}

func (s *Service) restoreVolumeWorkspaceScopeInternal(ctx context.Context, containerID string, backup *volumeWorkspaceBackupInternal) error {
	removePaths := volumeWorkspaceRollbackPathsInternal(backup)
	args := append([]string{"sh", "-c", `set -e
for rel do rm -rf -- "/volume/$rel"; done`, "sh"}, removePaths...)
	if _, stderr, err := s.deps.Exec(ctx, containerID, "", args); err != nil {
		return classifyVolumeWorkspaceExecErrorInternal(err, stderr, "remove failed volume workspace changes")
	}

	for _, archive := range backup.archives {
		parent := path.Dir(archive.relativePath)
		containerParent := "/volume"
		if parent != "." {
			containerParent = path.Join(containerParent, parent)
		}
		if _, stderr, err := s.deps.Exec(ctx, containerID, "", []string{"mkdir", "-p", containerParent}); err != nil {
			return classifyVolumeWorkspaceExecErrorInternal(err, stderr, "recreate volume workspace backup parent")
		}
		if _, stderr, err := s.deps.Exec(ctx, containerID, "", []string{"tar", "-xf", archive.archivePath, "-C", containerParent}); err != nil {
			return classifyVolumeWorkspaceExecErrorInternal(err, stderr, "restore volume workspace backup")
		}
	}
	return nil
}

func (s *Service) restoreVolumeWorkspaceFileInternal(ctx context.Context, containerID, volumeName, backupID, rel string) error {
	if err := s.validateVolumeWorkspacePathInternal(ctx, containerID, rel, true); err != nil {
		return err
	}
	filename, cleaned, err := s.deps.ArchiveFileTarget(ctx, volumeName, backupID, rel)
	if err != nil {
		return err
	}
	stderr, err := s.deps.RestoreArchiveMembers(ctx, containerID, filename, []string{cleaned})
	if err != nil {
		return classifyVolumeWorkspaceExecErrorInternal(err, stderr, "restore volume workspace file")
	}
	if strings.TrimSpace(stderr) != "" {
		slog.DebugContext(ctx, "volume workspace change stderr", "operation", volume.FileOpRestoreFile, "path", rel, "stderr", strings.TrimSpace(stderr))
	}
	return nil
}

func normalizeVolumeWorkspaceScopeInternal(paths []string) []string {
	normalized := slices.Clone(paths)
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)
	result := make([]string, 0, len(normalized))
	for _, candidate := range normalized {
		if candidate == "." || candidate == "" {
			continue
		}
		if slices.ContainsFunc(result, func(parent string) bool { return kit.FilePathMatches(candidate, parent) }) {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func volumeWorkspaceBackupScopeInternal(changes []volume.WorkspaceFileChange) ([]string, error) {
	paths := make([]string, 0, len(changes)*2)
	for _, change := range changes {
		rel, err := kit.NormalizeRelativePath(change.RelativePath)
		if err != nil {
			return nil, err
		}
		paths = append(paths, rel)
		switch change.Operation {
		case volume.FileOpRename:
			newName, validateFileNameErr := kit.ValidateFileName(change.NewName)
			if validateFileNameErr != nil {
				return nil, validateFileNameErr
			}
			paths = append(paths, path.Join(path.Dir(rel), newName))
		case volume.FileOpMove:
			parent := ""
			if strings.TrimSpace(change.NewParentPath) != "" {
				parent, err = kit.NormalizeRelativePath(change.NewParentPath)
				if err != nil {
					return nil, err
				}
			}
			paths = append(paths, path.Join(parent, path.Base(rel)))
		}
	}
	return normalizeVolumeWorkspaceScopeInternal(paths), nil
}

// The read is bounded by baseline length plus one, so unequal longer files
// conflict without loading their remaining bytes. The tree guard runs first.
func (s *Service) checkVolumeWorkspaceBaselineInternal(ctx context.Context, containerID, relativePath string, baseline []byte) error {
	rel, err := kit.NormalizeRelativePath(relativePath)
	if err != nil {
		return common.Classify(common.ErrVolumeWorkspaceBadRequest, err)
	}
	limit := int64(len(baseline)) + 1
	stdout, stderr, err := s.deps.Exec(ctx, containerID, "", []string{"acfs", "read", "--root", "/volume", "--path", "/" + rel, "--limit", strconv.FormatInt(limit, 10)})
	if err != nil {
		return classifyVolumeWorkspaceExecErrorInternal(err, stderr, "read volume workspace baseline")
	}
	reader := strings.NewReader(stdout)
	size, err := acfs.ReadStreamHeader(reader)
	if err != nil {
		return fmt.Errorf("parse volume workspace baseline header: %w", err)
	}
	if size > uint64(limit) || uint64(reader.Len()) != size {
		return errors.New("invalid volume workspace baseline payload")
	}
	current, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, baseline) {
		return common.Classify(common.ErrVolumeWorkspaceConflict, errors.New("file content changed since it was loaded"))
	}
	return nil
}
