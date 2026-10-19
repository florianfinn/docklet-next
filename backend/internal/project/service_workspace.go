// Source: getarcaneapp/arcane/backend/internal/project/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: SQLite service transaction policy; split the adopted service to keep files below 1,000 lines; local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite-only local persistence.

package project

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	projectdetails "github.com/florianfinn/docklet-next/backend/internal/project/children/details"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/listing"
	projectsync "github.com/florianfinn/docklet-next/backend/internal/project/children/sync"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/tags"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/update"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	workspacepkg "github.com/florianfinn/docklet-next/backend/pkg/workspace"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	workspacetypes "github.com/florianfinn/docklet-next/types/workspace"
	"github.com/moby/moby/api/types/container"
	"github.com/samber/mo"
	"go.getarcane.app/acfs"
	acfstypes "go.getarcane.app/acfs/types"
	"go.getarcane.app/sys/cgroup"
	updatertypes "go.getarcane.app/updater/types"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func (s *ProjectService) UpdateProject(ctx context.Context, projectID string, name, composeContent, envContent, overrideContent *string, user usertypes.Actor) (*Project, error) {
	proj, projectsDirectory, err := s.getProjectForUpdate(ctx, projectID)
	if err != nil {
		return nil, err
	}

	name = resolveAuthoritativeProjectName(ctx, &proj, name, composeContent)
	newName := strings.TrimSpace(mo.PointerToOption(name).OrEmpty())
	if recoverErr := s.updates.RecoverProject(ctx, projectID); recoverErr != nil {
		if newName != "" && proj.Name != newName {
			return nil, recoverErr
		}
		slog.WarnContext(ctx, "project rename journal recovery failed before non-rename update; continuing", "projectId", projectID, "error", recoverErr)
	} else {
		proj, projectsDirectory, recoverErr = s.getProjectForUpdate(ctx, projectID)
		if recoverErr != nil {
			return nil, recoverErr
		}
		name = resolveAuthoritativeProjectName(ctx, &proj, name, composeContent)
		newName = strings.TrimSpace(mo.PointerToOption(name).OrEmpty())
	}
	renameRequested := newName != "" && proj.Name != newName

	if proj.IsArchived {
		return nil, common.Classify(common.ErrProjectArchived, errors.New("project is archived and must be unarchived before this action"))
	}
	if ensureProjectEnvReadableErr := update.EnsureProjectEnvReadable(ctx, projectsDirectory, proj.Path); ensureProjectEnvReadableErr != nil {
		return nil, ensureProjectEnvReadableErr
	}

	// A rename requires the stored and the live status to both be stopped.
	var volumeMigration volume.Migration
	if renameRequested {
		if proj.Status != ProjectStatusStopped && proj.Status != ProjectStatusUnknown {
			return nil, fmt.Errorf("project must be stopped before renaming (current status: %s)", proj.Status)
		}
		services, servicesErr := s.projectServices(ctx, proj.ID)
		if servicesErr != nil {
			slog.WarnContext(ctx, "failed to resolve project status before rename", "projectId", proj.ID, "error", servicesErr)
			return nil, fmt.Errorf("project must be stopped before renaming (current status: %s): failed to verify live status: %w", proj.Status, servicesErr)
		}
		if status := ProjectStatus(listing.ProjectStatus(services)); status != ProjectStatusStopped {
			return nil, fmt.Errorf("project must be stopped before renaming (current status: %s)", status)
		}
		proj.Status, proj.StatusReason = ProjectStatusStopped, nil
		proj.ServiceCount, proj.RunningCount = listing.ServiceCounts(services)

		if volumeMigration, err = s.prepareProjectRenameVolumeMigration(ctx, &proj, newName, projectsDirectory, composeContent, envContent, overrideContent); err != nil {
			return nil, err
		}
	}

	renameJournal := s.updates.Prepare(proj.ID, proj.Name, proj.Path, proj.DirName, name, projectsDirectory, volumeMigration)

	contentChanged := composeContent != nil || envContent != nil || overrideContent != nil
	var backup *projects.ProjectUpdateBackup
	if contentChanged {
		var cleanupBackup func()
		backup, cleanupBackup, err = projects.BackupProjectDirectory(ctx, projectsDirectory, proj.Path, ".project-update-backup-*", projects.ProjectUpdateBackupScope{TopLevelFiles: true})
		if err != nil {
			return nil, err
		}
		defer cleanupBackup()
	}

	journalActive := renameJournal != nil
	if journalActive {
		if writeProjectRenameJournalErr := s.updates.WriteJournal(ctx, renameJournal, projecttypes.RenameJournalPhaseStarted); writeProjectRenameJournalErr != nil {
			return nil, writeProjectRenameJournalErr
		}
	}

	projectStateCommitted := false
	if updateErr := withProjectRenameRollback(ctx, &proj, &projectStateCommitted, func() error {
		return s.applyProjectUpdate(
			ctx, &proj, renameRequested, newName, projectsDirectory,
			composeContent, envContent, overrideContent,
			volumeMigration, renameJournal, &journalActive, &projectStateCommitted,
		)
	}); updateErr != nil {
		return nil, s.handleProjectUpdateFailure(ctx, projectID, projectsDirectory, &proj, backup, &journalActive, projectStateCommitted, updateErr)
	}

	if composeContent != nil || overrideContent != nil {
		s.refreshProjectAfterContentUpdate(ctx, &proj)
	}
	metadata := database.JSON{"action": "update", "projectID": proj.ID, "projectName": proj.Name}
	if composeContent != nil {
		metadata["composeUpdated"] = true
	}
	if envContent != nil {
		metadata["envUpdated"] = true
	}
	if overrideContent != nil {
		metadata["overrideUpdated"] = true
	}
	s.logProjectEvent(ctx, event.EventTypeProjectUpdate, proj.ID, proj.Name, user, metadata, "could not log project update action")
	if contentChanged {
	}

	slog.InfoContext(ctx, "project updated", "projectId", proj.ID, "name", proj.Name)
	return &proj, nil
}

// applyProjectUpdate renames, persists files, migrates volumes, and
// saves the row; a failure before the save rolls an applied volume migration back.
func (s *ProjectService) applyProjectUpdate(
	ctx context.Context,
	proj *Project,
	renameRequested bool,
	newName, projectsDirectory string,
	composeContent, envContent, overrideContent *string,
	volumeMigration volume.Migration,
	renameJournal *projecttypes.RenameJournal,
	journalActive, projectStateCommitted *bool,
) (err error) {
	volumeMigrationApplied := false
	defer func() {
		if err != nil && volumeMigrationApplied && !*projectStateCommitted {
			if rollbackErr := volumeMigration.Rollback(ctx); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("failed to rollback project volume rename: %w", rollbackErr))
			}
		}
	}()

	if renameRequested {
		if renameErr := s.applyProjectRename(ctx, proj, newName, projectsDirectory); renameErr != nil {
			return renameErr
		}
	}
	if persistFilesErr := s.persistUpdatedProjectFiles(ctx, proj, projectsDirectory, composeContent, envContent, overrideContent); persistFilesErr != nil {
		return persistFilesErr
	}
	if migrationErr := projects.ApplyRenameVolumeMigration(ctx, s.updates.Operations(), volumeMigration, renameJournal, &volumeMigrationApplied); migrationErr != nil {
		return migrationErr
	}
	if saveErr := s.db.WithContext(ctx).Save(proj).Error; saveErr != nil {
		return fmt.Errorf("failed to update project: %w", saveErr)
	}
	*projectStateCommitted = true
	projects.FinalizeRenameAfterCommit(ctx, s.updates.Operations(), proj.ID, volumeMigration, renameJournal, journalActive)
	return nil
}

// handleProjectUpdateFailure restores the pre-update files and replays
// an active rename journal unless the project row was already committed.
func (s *ProjectService) handleProjectUpdateFailure(
	ctx context.Context,
	projectID, projectsDirectory string,
	proj *Project,
	backup *projects.ProjectUpdateBackup,
	journalActive *bool,
	projectStateCommitted bool,
	err error,
) error {
	if projectStateCommitted {
		return err
	}

	if backup != nil {
		if restoreErr := projects.RestoreProjectDirectoryBackup(ctx, projectsDirectory, proj.Path, backup); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to restore project files after update failure: %w", restoreErr))
		}
	}
	if *journalActive {
		if recoverErr := s.updates.RecoverProject(ctx, projectID); recoverErr != nil {
			err = errors.Join(err, fmt.Errorf("project rename recovery failed: %w", recoverErr))
		} else {
			*journalActive = false
		}
	}
	return err
}

// refreshProjectAfterContentUpdate re-derives the compose name, image
// refs, Compose tags, and service counts after a compose or override write.
func (s *ProjectService) refreshProjectAfterContentUpdate(ctx context.Context, proj *Project) {
	s.refreshComposeProjectName(ctx, proj)
	s.refreshProjectImageRefs(ctx, proj)
	if err := s.reconcileComposeTagsForProject(ctx, proj); err != nil {
		slog.WarnContext(ctx, "failed to reconcile Compose project tags after project content update", "projectId", proj.ID, "error", err)
	}
	if err := s.updateProjectStatusAndCounts(ctx, proj.ID, proj.Status); err != nil {
		slog.WarnContext(ctx, "failed to update service counts after project content update", "projectId", proj.ID, "error", err)
	}
}

func (s *ProjectService) getProjectForUpdate(ctx context.Context, projectID string) (Project, string, error) {
	var proj Project
	if err := s.db.WithContext(ctx).First(&proj, "id = ?", projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, "", errors.New("project not found")
		}
		return Project{}, "", fmt.Errorf("failed to get project: %w", err)
	}

	projectsDirectory, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		return Project{}, "", fmt.Errorf("failed to get projects directory: %w", err)
	}

	if ensureProjectPathUnderRootErr := s.EnsureProjectPathUnderRoot(ctx, &proj, false); ensureProjectPathUnderRootErr != nil {
		return Project{}, "", ensureProjectPathUnderRootErr
	}

	return proj, projectsDirectory, nil
}

// prepareProjectRenameVolumeMigration plans the volume rename for a
// stopped project. Content changes are applied to a scratch preview first so
// the plan reflects the updated compose files.
func (s *ProjectService) prepareProjectRenameVolumeMigration(
	ctx context.Context,
	proj *Project,
	newName, projectsDirectory string,
	composeContent, envContent, overrideContent *string,
) (volume.Migration, error) {
	target := proj
	if composeContent != nil || envContent != nil || overrideContent != nil {
		previewLogical, err := acfs.MkdirTemp(ctx, projectsDirectory, "/", ".project-update-preview-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create project update preview: %w", err)
		}
		previewPath := filepath.Join(projectsDirectory, filepath.FromSlash(strings.TrimPrefix(previewLogical, "/")))
		defer func() {
			// acfs refuses an already-cancelled context, so cleanup runs detached
			// or a cancelled update leaks the preview directory.
			cleanupCtx := context.WithoutCancel(ctx)
			if removeErr := acfs.RemoveAll(cleanupCtx, projectsDirectory, previewLogical); removeErr != nil {
				slog.WarnContext(cleanupCtx, "failed to remove project update preview", "path", previewPath, "error", removeErr)
			}
		}()

		if _, copyDirErr := acfs.CopyDir(ctx, proj.Path, previewPath, acfstypes.CopyOptions{}); copyDirErr != nil {
			return nil, fmt.Errorf("failed to prepare project update preview: %w", copyDirErr)
		}
		previewProject := *proj
		previewProject.Path = previewPath
		if persistErr := s.persistUpdatedProjectFiles(ctx, &previewProject, projectsDirectory, composeContent, envContent, overrideContent); persistErr != nil {
			return nil, fmt.Errorf("failed to prepare project update preview: %w", persistErr)
		}
		target = &previewProject
	}

	oldComposeName, newComposeName := projects.NormalizeProjectName(target.Name), projects.NormalizeProjectName(newName)
	if s.dockerService == nil || target.Status != ProjectStatusStopped || oldComposeName == "" || newComposeName == "" || oldComposeName == newComposeName {
		return nil, nil
	}

	composeProject, _, err := s.loadComposeProjectForProject(ctx, target, nil)
	if err != nil {
		if errors.Is(err, common.ErrProjectComposeFileNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load compose project for volume rename: %w", err)
	}

	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker for volume rename: %w", err)
	}

	toolsRegistry := ""
	if s.settingsService != nil {
		toolsRegistry = s.settingsService.GetSettingsConfig().ToolsImageRegistry.Value
	}

	return projects.PlanVolumeMigration(ctx, dockerClient, composeProject, oldComposeName, newComposeName, volumehelper.ToolsImage(toolsRegistry))
}

// persistUpdatedProjectFiles validates and writes submitted compose, override,
// and env content. An override-only save validates the on-disk base merged with
// the requested override, so a base only valid with its override still passes
// and a delete that would break the base fails before touching disk.
func (s *ProjectService) persistUpdatedProjectFiles(ctx context.Context, proj *Project, projectsDirectory string, composeContent, envContent, overrideContent *string) error {
	if composeContent == nil && overrideContent == nil {
		if envContent != nil {
			return projectsync.PersistEffectiveEnvContent(ctx, proj.Path, projectsDirectory, *envContent)
		}
		return nil
	}

	var baseContent string
	if composeContent != nil {
		baseContent = *composeContent
	} else {
		var err error
		if baseContent, _, err = projects.ReadProjectFiles(ctx, proj.Path, ""); err != nil {
			return fmt.Errorf("failed to read project files: %w", err)
		}
	}
	effectiveEnvContent, err := projectsync.EffectiveEnvContentForUpdate(proj.Path, envContent)
	if err != nil {
		return fmt.Errorf("invalid compose file: %w", err)
	}
	valOverride, valOverrideName := projects.ResolveEffectiveOverrideForValidation(proj.Path, overrideContent)
	if validateErr := projects.ValidateComposeContentForUpdate(ctx, projectsDirectory, proj.Path, proj.Name, baseContent, effectiveEnvContent, valOverride, valOverrideName, false); validateErr != nil {
		return fmt.Errorf("invalid compose file: %w", validateErr)
	}

	// The env is persisted first so WriteComposeFile targets the COMPOSE_FILE
	// base the updated .env selects. A non-nil composeContent is an explicit
	// submission and is always written.
	if envContent != nil {
		if persistErr := projectsync.PersistEffectiveEnvContent(ctx, proj.Path, projectsDirectory, *envContent); persistErr != nil {
			return fmt.Errorf("failed to save project files: %w", persistErr)
		}
	} else if composeContent != nil {
		if ensureErr := projectsync.EnsureEffectiveEnvFile(ctx, proj.Path, projectsDirectory); ensureErr != nil {
			return fmt.Errorf("failed to save project files: %w", ensureErr)
		}
	}
	if composeContent != nil {
		if writeErr := projects.WriteComposeFile(ctx, projectsDirectory, proj.Path, *composeContent); writeErr != nil {
			return fmt.Errorf("failed to save project files: %w", writeErr)
		}
	}
	if overrideErr := projects.ApplyOverrideFileChange(ctx, projectsDirectory, proj.Path, overrideContent); overrideErr != nil {
		return fmt.Errorf("failed to save project files: %w", overrideErr)
	}
	return nil
}

// applyProjectRename moves a stopped project's directory to its sanitized new
// name under the projects directory and updates the row fields.
func (s *ProjectService) applyProjectRename(ctx context.Context, proj *Project, newName, projectsDirectory string) error {
	if proj.Status != ProjectStatusStopped {
		return fmt.Errorf("project must be stopped before renaming (current status: %s)", proj.Status)
	}

	newDirName := projects.SanitizeProjectName(newName)
	if newDirName == "" || strings.Trim(newDirName, "_") == "" {
		return errors.New("invalid project name: results in empty directory name")
	}

	currentPath := filepath.Clean(proj.Path)
	targetPath := filepath.Clean(filepath.Join(projectsDirectory, newDirName))
	if currentPath != targetPath {
		targetLogical, err := acfs.LogicalPath(projectsDirectory, targetPath)
		if err != nil {
			return fmt.Errorf("failed to resolve project directory rename target: %w", err)
		}
		exists, err := acfs.Exists(ctx, projectsDirectory, targetLogical)
		if err != nil {
			return fmt.Errorf("failed to check project directory rename target: %w", err)
		}
		if exists {
			return fmt.Errorf("project directory already exists: %s", targetPath)
		}

		// An imported project can live outside the projects directory, in which
		// case the move crosses roots and cannot be a confined rename.
		currentLogical, currentErr := acfs.LogicalPath(projectsDirectory, currentPath)
		if currentErr != nil {
			// The cross-root move cannot go through acfs, so the cancellation
			// check acfs.Rename performs happens here instead.
			if cancellationErr := ctx.Err(); cancellationErr != nil {
				return cancellationErr
			}
			err = os.Rename(currentPath, targetPath)
		} else {
			err = acfs.Rename(ctx, projectsDirectory, currentLogical, targetLogical)
		}
		if err != nil {
			return fmt.Errorf("failed to rename project directory: %w", err)
		}

		proj.Path = targetPath
	}

	proj.DirName = &newDirName
	proj.Name = newName
	return nil
}

// UpdateProjectServiceImages persists selected service image tags before recreating them.
func (s *ProjectService) UpdateProjectServiceImages(ctx context.Context, projectID string, changes map[string]updatertypes.ServiceImageChange, user usertypes.Actor) error {
	services, err := s.SaveProjectServiceImages(ctx, projectID, changes)
	if err != nil {
		return err
	}
	// Keep the desired source on deployment failure: Compose may have partially
	// recreated services, and the pending update must remain retryable.
	return s.UpdateProjectServices(ctx, projectID, services, user, false)
}

// SaveProjectServiceImages persists selected service image tags without
// deploying them and returns the changed services.
func (s *ProjectService) SaveProjectServiceImages(ctx context.Context, projectID string, changes map[string]updatertypes.ServiceImageChange) ([]string, error) {
	if len(changes) == 0 {
		return nil, errors.New("service image changes are required")
	}
	proj, err := s.getMutableProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	// Selecting the changed services keeps profile-gated ones active.
	effective, _, err := s.loadComposeProjectForProject(ctx, proj, nil, slices.Collect(maps.Keys(changes))...)
	if err != nil {
		return nil, fmt.Errorf("load project for tag update: %w", err)
	}
	services, err := s.updates.ApplyImageChanges(ctx, proj.Path, effective, changes)
	if err != nil {
		return nil, err
	}
	s.invalidateProjectCaches(projectID)
	return services, nil
}

func (s *ProjectService) GetProjectWorkspace(ctx context.Context, projectID string) (*workspacetypes.Workspace, error) {
	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if ensureProjectPathUnderRootErr := s.EnsureProjectPathUnderRoot(ctx, proj, false); ensureProjectPathUnderRootErr != nil {
		return nil, ensureProjectPathUnderRootErr
	}
	return s.workspace.Read(ctx, proj.Path, s.workspaceComposeFileName(ctx, proj))
}

func (s *ProjectService) GetProjectWorkspaceFile(ctx context.Context, projectID, relativePath string) (*workspacetypes.FileContent, error) {
	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return s.workspace.File(ctx, proj.Path, s.workspaceComposeFileName(ctx, proj), relativePath)
}

func (s *ProjectService) DownloadProjectWorkspaceFile(ctx context.Context, projectID, relativePath string) (io.ReadCloser, int64, string, error) {
	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return nil, 0, "", err
	}
	return s.workspace.Download(ctx, proj.Path, s.workspaceComposeFileName(ctx, proj), relativePath)
}

func (s *ProjectService) UpdateProjectWorkspace(
	ctx context.Context,
	projectID string,
	manifest projecttypes.WorkspaceUpdateManifest,
	uploads map[int][]byte,
	user usertypes.Actor,
) (*workspacetypes.Workspace, error) {
	if err := workspacepkg.ValidateUpdateManifest(manifest.FileTreeRevision, len(manifest.FileChanges), 500); err != nil {
		return nil, common.Classify(common.ErrProjectWorkspaceBadRequest, err)
	}
	proj, err := s.getMutableProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if ensureProjectPathUnderRootErr := s.EnsureProjectPathUnderRoot(ctx, proj, true); ensureProjectPathUnderRootErr != nil {
		return nil, ensureProjectPathUnderRootErr
	}

	projectsDirectory, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		return nil, err
	}
	if ensureProjectEnvReadableErr := update.EnsureProjectEnvReadable(ctx, projectsDirectory, proj.Path); ensureProjectEnvReadableErr != nil {
		return nil, ensureProjectEnvReadableErr
	}
	if applyErr := s.workspace.Apply(ctx, projectsDirectory, proj.Path, s.workspaceComposeFileName(ctx, proj), manifest, uploads); applyErr != nil {
		return nil, applyErr
	}

	s.refreshProjectImageRefs(ctx, proj)
	if updateProjectStatusandCountsErr := s.updateProjectStatusAndCounts(ctx, proj.ID, proj.Status); updateProjectStatusandCountsErr != nil {
		return nil, fmt.Errorf("refresh project after workspace update: %w", updateProjectStatusandCountsErr)
	}
	s.logProjectEvent(ctx, event.EventTypeProjectUpdate, proj.ID, proj.Name, user, database.JSON{
		"action":          "update_project_workspace",
		"fileChangeCount": len(manifest.FileChanges),
	}, "could not log project workspace update")
	return s.GetProjectWorkspace(ctx, projectID)
}

func (s *ProjectService) workspaceComposeFileName(ctx context.Context, proj *Project) string {
	if composeFile, err := s.ResolveProjectComposeFile(ctx, proj); err == nil {
		return filepath.Base(composeFile)
	}
	return projects.DefaultComposeFileName
}

func (s *ProjectService) projectServices(ctx context.Context, projectID string) ([]projecttypes.RuntimeService, error) {
	projectFromDb, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	composeProject, composeFileFullPath, derr := s.loadComposeProjectForProject(ctx, projectFromDb, nil)
	if errors.Is(derr, common.ErrProjectEnvUnreadable) {
		// The Compose file cannot be loaded, so derive services from labeled containers.
		containers, listErr := s.details.ComposeContainers(ctx)
		if listErr != nil {
			return nil, listErr
		}
		meta := s.ProjectMetadata(ctx, *projectFromDb, nil)
		matched, ownerErr := s.projectContainers(ctx, *projectFromDb, containers)
		if ownerErr != nil {
			return nil, ownerErr
		}
		currentContainerID, currentContainerErr := cgroup.CurrentContainerID()
		services := make([]projecttypes.RuntimeService, 0, len(matched))
		for _, c := range matched {
			services = append(services, listing.RuntimeServiceFromContainer(IconCatalogForContext(ctx), c, meta, currentContainerID, currentContainerErr))
		}
		return services, nil
	}
	if derr != nil {
		return []projecttypes.RuntimeService{}, fmt.Errorf("failed to load compose project in %s: %w", projectFromDb.Path, derr)
	}

	projectsDirectory, projectsDirErr := s.GetProjectsDirectory(ctx)
	if projectsDirErr != nil {
		slog.WarnContext(ctx, "failed to resolve projects directory for Arcane compose metadata", "path", composeFileFullPath, "error", projectsDirErr)
	}
	autoInjectEnv := s.settingsService.GetBoolSetting(ctx, "autoInjectEnv", false)
	return s.details.ComposeServices(ctx, composeProject, composeFileFullPath, projectsDirectory, autoInjectEnv, IconCatalogForContext(ctx))
}

func (s *ProjectService) StreamProjectLogs(ctx context.Context, projectID string, logsChan chan<- string, follow bool, tail, since string, timestamps bool) error {
	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return err
	}
	return projectdetails.Logs(ctx, observedComposeName(*proj), logsChan, follow, tail, since, timestamps)
}

func (s *ProjectService) GetProjectStatusCounts(ctx context.Context) (projecttypes.StatusCounts, error) {
	var projectsList []Project
	if listProjectsErr := s.db.WithContext(ctx).Find(&projectsList).Error; listProjectsErr != nil {
		return projecttypes.StatusCounts{}, fmt.Errorf("failed to list projects: %w", listProjectsErr)
	}
	return s.listing.StatusCounts(ctx, projectRecords(projectsList)), nil
}

// projectListRows builds list rows and persists live-inferred service counts
// over stored zeros, since SQL sorts and paginates on service_count.
func (s *ProjectService) projectListRows(ctx context.Context, projectsDir string, projectsList []Project, snapshot listing.Snapshot, ownership ...map[string]string) []projecttypes.Details {
	rows, inferredCounts := listing.Rows(projectsDir, IconCatalogForContext(ctx), projectRecords(projectsList), snapshot, ownership...)
	ids := slices.Collect(maps.Keys(inferredCounts))
	for chunk := range slices.Chunk(ids, inferredServiceCountBatchSize) {
		var caseExpr strings.Builder
		args := make([]any, 0, 2*len(chunk))
		caseExpr.WriteString("CASE id")
		for _, id := range chunk {
			caseExpr.WriteString(" WHEN ? THEN ?")
			args = append(args, id, inferredCounts[id])
		}
		caseExpr.WriteString(" ELSE service_count END")
		if err := s.db.WithContext(ctx).Model(&Project{}).
			Where("id IN ? AND service_count = 0", chunk).
			Update("service_count", gorm.Expr(caseExpr.String(), args...)).Error; err != nil {
			slog.WarnContext(ctx, "failed to persist inferred project service counts", "count", len(chunk), "error", err)
			break
		}
	}
	return rows
}

// GetProjectTags returns the effective UI and Compose tag associations for a project.
func (s *ProjectService) GetProjectTags(ctx context.Context, projectID string) ([]projecttypes.Tag, error) {
	var rows []projecttypes.TagAssignment
	if err := s.db.WithContext(ctx).Model(&ProjectTag{}).Where("project_id = ?", projectID).Order("name, source, color").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load project tags: %w", err)
	}
	return tags.Group(rows)[projectID], nil
}

// ListProjectTagOptions returns the distinct tag names and colors available in the current environment.
func (s *ProjectService) ListProjectTagOptions(ctx context.Context) ([]projecttypes.TagOption, error) {
	var rows []projecttypes.TagAssignment
	if err := s.db.WithContext(ctx).Model(&ProjectTag{}).Order("name, source DESC, color").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list project tag options: %w", err)
	}
	// Rows are ordered by name with the UI source first, so each name's first row wins.
	options := make([]projecttypes.TagOption, 0)
	for _, row := range rows {
		if len(options) == 0 || options[len(options)-1].Name != row.Name {
			options = append(options, projecttypes.TagOption{Name: row.Name, Color: row.Color})
		}
	}
	return options, nil
}

// ListProjectReferences returns the ID and name of every project, including archived ones, without runtime state.
func (s *ProjectService) ListProjectReferences(ctx context.Context) ([]projecttypes.Reference, error) {
	references := []projecttypes.Reference{}
	if err := s.db.WithContext(ctx).Model(&Project{}).Select("id", "name").Order("name").Find(&references).Error; err != nil {
		return nil, fmt.Errorf("list project references: %w", err)
	}
	return references, nil
}

// UpdateProjectTag attaches or detaches a UI-managed tag and rejects Compose-owned names.
func (s *ProjectService) UpdateProjectTag(ctx context.Context, projectID, name string, color projecttypes.TagColor, attached bool, user usertypes.Actor) ([]projecttypes.Tag, error) {
	normalized, normalizedColor, err := tags.NormalizeUpdate(name, color, attached)
	if err != nil {
		return nil, err
	}

	var projectModel Project
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if findProjectErr := tx.First(&projectModel, "id = ?", projectID).Error; findProjectErr != nil {
			return fmt.Errorf("find project for tag update: %w", findProjectErr)
		}
		return tags.ApplyUpdate(tagStore{tx: tx}, projectID, normalized, normalizedColor, attached)
	})
	if err != nil {
		return nil, err
	}

	metadata := database.JSON{"action": "update_tags", "projectID": projectID, "projectName": projectModel.Name, "tag": normalized, "attached": attached}
	s.logProjectEvent(ctx, event.EventTypeProjectUpdate, projectID, projectModel.Name, user, metadata, "could not log project tag update")
	return s.GetProjectTags(ctx, projectID)
}

func (s *ProjectService) reconcileComposeProjectTags(ctx context.Context, projectID string, composeTags []projecttypes.TagOption) error {
	normalized, err := tags.NormalizeComposeProjectTags(composeTags)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var projectModel Project
		if findProjectErr := tx.Select("id").First(&projectModel, "id = ?", projectID).Error; findProjectErr != nil {
			return fmt.Errorf("find project for Compose tag reconciliation: %w", findProjectErr)
		}
		return tags.ReplaceCompose(tagStore{tx: tx}, projectID, normalized)
	})
}

// projectComposeOwners reads all identities so filters/pages cannot promote aliases.
func (s *ProjectService) projectComposeOwners(ctx context.Context) (map[string]string, []projecttypes.Record, error) {
	var records []Project
	if err := s.db.WithContext(ctx).Select("id", "name", "compose_project_name").Find(&records).Error; err != nil {
		return nil, nil, fmt.Errorf("load Compose project owners: %w", err)
	}
	identities := projectRecords(records)
	return listing.ComposeProjectOwners(identities), identities, nil
}

func (s *ProjectService) projectContainers(ctx context.Context, p Project, containers []container.Summary) ([]container.Summary, error) {
	owners, _, err := s.projectComposeOwners(ctx)
	if err != nil {
		return nil, err
	}
	return listing.ProjectContainers(projectRecord(p), listing.GroupComposeContainersByProject(containers), owners), nil
}
