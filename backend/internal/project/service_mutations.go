// Source: getarcaneapp/arcane/backend/internal/project/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: SQLite service transaction policy; split the adopted service to keep files below 1,000 lines; local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite-only local persistence.; M14 container and host operation contracts.

package project

import (
	"context"
	"errors"
	"fmt"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	projectdetails "github.com/florianfinn/docklet-next/backend/internal/project/children/details"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/listing"
	projectsync "github.com/florianfinn/docklet-next/backend/internal/project/children/sync"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/tags"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/workspace"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/iconcatalog"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	workspacepkg "github.com/florianfinn/docklet-next/backend/pkg/workspace"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/samber/mo"
	"go.getarcane.app/acfs"
	dockertypes "go.getarcane.app/docker/types"
	kit "go.getarcane.app/kit/pkg"
	"go.getarcane.app/kit/pkg/mapping"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"
	"go.getarcane.app/updater/pkg/utils/tagpolicy"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func (s *ProjectService) DownProject(ctx context.Context, projectID string, user usertypes.Actor) error {
	projectFromDb, err := s.getMutableProject(ctx, projectID)
	if err != nil {
		return err
	}

	proj, _, lerr := s.loadComposeProjectForProject(ctx, projectFromDb, nil)
	if lerr != nil {
		return fmt.Errorf("failed to load compose project: %w", lerr)
	}

	defer s.eventService.BeginComposeSuppressionWindow(proj.Name)()

	if composeDownErr := composeDownProject(ctx, proj, false); composeDownErr != nil {
		return fmt.Errorf("failed to bring down project: %w", composeDownErr)
	}

	metadata := database.JSON{
		"action":      "down",
		"projectID":   projectID,
		"projectName": projectFromDb.Name,
	}
	s.logProjectEvent(ctx, event.EventTypeProjectStop, projectID, projectFromDb.Name, user, metadata, "could not log project down action")

	return s.updateProjectStatusAndCounts(ctx, projectID, ProjectStatusStopped)
}

// CreateProject creates a project's directory, files, and DB row. A directory
// collision appends "-N" unless allowNameSuffix is false, which returns projects.ErrProjectDirExists.
func (s *ProjectService) CreateProject(
	ctx context.Context,
	name, composeContent string,
	envContent *string,
	manifest projecttypes.CreateProjectWorkspaceManifest,
	uploads map[int][]byte,
	uiTags []string,
	uiTagColors map[string]projecttypes.TagColor,
	user usertypes.Actor,
	allowNameSuffixOptions ...bool,
) (*Project, error) {
	normalizedUITags, err := projects.NormalizeProjectTags(uiTags)
	if err != nil {
		return nil, fmt.Errorf("invalid project tags: %w", err)
	}
	normalizedTagColors, err := tags.NormalizeProjectTagColors(uiTagColors)
	if err != nil {
		return nil, fmt.Errorf("invalid project tag colors: %w", err)
	}
	allowNameSuffix := true
	if len(allowNameSuffixOptions) > 0 {
		allowNameSuffix = allowNameSuffixOptions[0]
	}
	// A top-level `name:` in the compose file is authoritative over the
	// submitted project name.
	if yamlName := projects.ComposeContentProjectName(composeContent); yamlName != "" {
		name = yamlName
	}
	sanitized := projects.SanitizeProjectName(name)

	projectsDirectory, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get projects directory: %w", err)
	}

	basePath := filepath.Join(projectsDirectory, sanitized)
	var projectPath, folderName string
	if allowNameSuffix {
		projectPath, folderName, err = projects.CreateUniqueDir(ctx, projectsDirectory, basePath, name, utils.DirPerm)
	} else {
		projectPath, folderName, err = projects.CreateExactDir(ctx, projectsDirectory, basePath, name, utils.DirPerm)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create project directory: %w", err)
	}
	projectLogical, err := acfs.LogicalPath(projectsDirectory, projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve created project directory: %w", err)
	}

	proj := &Project{
		Name:         name,
		DirName:      &folderName,
		Path:         projectPath,
		Status:       ProjectStatusStopped,
		ServiceCount: 0,
		RunningCount: 0,
	}

	if applyProjectWorkspaceChangesErr := projects.ApplyProjectWorkspaceChanges(ctx, projectPath, manifest.FileChanges, uploads, projects.ProjectWorkspaceApplyOptions{
		MaxDepth:         s.config.ProjectWorkspaceMaxDepth,
		MaxEntries:       s.config.ProjectWorkspaceMaxEntries,
		MaxFileSizeBytes: workspacepkg.MaxFileSizeBytes(s.config.ProjectWorkspaceMaxFileSizeMB),
		SkipDirectories:  s.config.ProjectScanSkipDirs,
		ComposeFileName:  projects.DefaultComposeFileName,
	}); applyProjectWorkspaceChangesErr != nil {
		_ = acfs.RemoveAll(context.WithoutCancel(ctx), projectsDirectory, projectLogical)
		return nil, workspace.WrapProjectWorkspaceError(applyProjectWorkspaceChangesErr)
	}

	// Creates without automatic name suffixes (allowNameSuffix=false) tolerate not-yet-supplied
	// ${VAR} references while the env file is still being supplied; interactive
	// creates (allowNameSuffix=true) stay strict.
	if validateComposeContentForUpdateErr := projects.ValidateComposeContentForUpdate(
		ctx,
		projectsDirectory,
		projectPath,
		name,
		composeContent,
		envContent,
		nil,
		"",
		!allowNameSuffix,
	); validateComposeContentForUpdateErr != nil {
		_ = acfs.RemoveAll(context.WithoutCancel(ctx), projectsDirectory, projectLogical)
		return nil, fmt.Errorf("invalid compose file: %w", validateComposeContentForUpdateErr)
	}

	if writeProjectFilesErr := projects.WriteProjectFiles(ctx, projectsDirectory, projectPath, composeContent, envContent); writeProjectFilesErr != nil {
		// Best-effort cleanup to restore pre-transaction behavior.
		_ = acfs.RemoveAll(context.WithoutCancel(ctx), projectsDirectory, projectLogical)
		return nil, fmt.Errorf("failed to save project files: %w", writeProjectFilesErr)
	}
	composeMeta, err := projects.ParseArcaneComposeMetadata(
		ctx,
		filepath.Join(projectPath, projects.DefaultComposeFileName),
		projectsDirectory,
		s.settingsService.GetBoolSetting(ctx, "autoInjectEnv", false),
	)
	if err != nil {
		slog.WarnContext(ctx, "failed to read Compose project tags during creation", "projectName", name, "error", err)
		composeMeta = projects.ArcaneComposeMetadata{}
	}
	// Compose owns the tags it declares, so they are not attached as UI tags.
	normalizedUITags = slices.DeleteFunc(normalizedUITags, func(tag string) bool {
		return slices.ContainsFunc(composeMeta.ProjectTags, func(composeTag projecttypes.TagOption) bool { return composeTag.Name == tag })
	})

	if transactionErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if createProjectErr := tx.Create(proj).Error; createProjectErr != nil {
			return createProjectErr
		}
		return tags.AttachInitial(tagStore{tx: tx}, proj.ID, normalizedUITags, normalizedTagColors)
	}); transactionErr != nil {
		_ = acfs.RemoveAll(context.WithoutCancel(ctx), projectsDirectory, projectLogical)
		return nil, fmt.Errorf("failed to create project: %w", transactionErr)
	}
	s.refreshComposeProjectName(ctx, proj)
	s.refreshProjectImageRefs(ctx, proj)
	if reconcileComposeProjectTagsErr := s.reconcileComposeProjectTags(ctx, proj.ID, composeMeta.ProjectTags); reconcileComposeProjectTagsErr != nil {
		cleanupCtx := context.WithoutCancel(ctx)
		databaseCleanupErr := s.db.WithContext(cleanupCtx).Transaction(func(tx *gorm.DB) error {
			return deleteProjectWithTags(tx, proj.ID)
		})
		fileCleanupErr := acfs.RemoveAll(cleanupCtx, projectsDirectory, projectLogical)
		if databaseCleanupErr != nil {
			databaseCleanupErr = fmt.Errorf("rollback project database state after tag reconciliation failure: %w", databaseCleanupErr)
		}
		if fileCleanupErr != nil {
			fileCleanupErr = fmt.Errorf("rollback project files after tag reconciliation failure: %w", fileCleanupErr)
		}
		return nil, errors.Join(fmt.Errorf("reconcile Compose project tags: %w", reconcileComposeProjectTagsErr), databaseCleanupErr, fileCleanupErr)
	}

	metadata := database.JSON{"action": "create", "projectID": proj.ID, "projectName": proj.Name, "path": projectPath}
	s.logProjectEvent(ctx, event.EventTypeProjectCreate, proj.ID, proj.Name, user, metadata, "could not log project creation")

	return proj, nil
}

func (s *ProjectService) DestroyProject(ctx context.Context, projectID string, removeFiles, removeVolumes bool, user usertypes.Actor) error {
	slog.DebugContext(ctx, "DestroyProject service called",
		"projectId", projectID,
		"removeFiles", removeFiles,
		"removeVolumes", removeVolumes,
		"userId", user.ID,
		"username", user.Username)

	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "Found project to destroy",
		"projectName", proj.Name,
		"projectPath", proj.Path)

	if downProjectErr := s.DownProject(ctx, projectID, usertypes.SystemUser); downProjectErr != nil {
		slog.WarnContext(ctx, "failed to bring down project", "error", downProjectErr)
	}

	if removeVolumes {
		if compProj, _, lerr := s.loadComposeProjectForProject(ctx, proj, nil); lerr == nil {
			defer s.eventService.BeginComposeSuppressionWindow(compProj.Name)()
			if derr := projects.ComposeDown(ctx, compProj, true); derr != nil {
				slog.WarnContext(ctx, "failed to remove volumes", "error", derr)
			}
		} else {
			slog.WarnContext(ctx, "failed to load compose project for volume removal", "error", lerr)
		}
	}

	if removeFiles {
		slog.DebugContext(ctx, "Removing project files", "path", proj.Path)
		// An imported project can live anywhere, so the removal is rooted at the
		// parent directory and names the project directory itself.
		if removeAllErr := acfs.RemoveAll(ctx, filepath.Dir(proj.Path), "/"+filepath.Base(proj.Path)); removeAllErr != nil {
			slog.ErrorContext(ctx, "Failed to remove project files", "path", proj.Path, "error", removeAllErr)
			return fmt.Errorf("failed to remove project files: %w", removeAllErr)
		}
		slog.InfoContext(ctx, "Project files removed successfully", "path", proj.Path)
	}

	if transactionErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return deleteProjectWithTags(tx, projectID)
	}); transactionErr != nil {
		return fmt.Errorf("failed to delete project from database: %w", transactionErr)
	}

	if !removeFiles {
		if projectsDir, dirErr := s.GetProjectsDirectory(ctx); dirErr != nil {
			slog.WarnContext(ctx, "Failed to resolve projects directory for quarantine", "error", dirErr)
		} else if projects.IsSafeSubdirectory(projectsDir, proj.Path) && filepath.Clean(projectsDir) != filepath.Clean(proj.Path) {
			trashName := fmt.Sprintf("%s%s-%d", projects.ArcaneTrashPrefix, filepath.Base(proj.Path), time.Now().Unix())
			trashPath := filepath.Join(filepath.Dir(proj.Path), trashName)
			if renameErr := acfs.Rename(ctx, filepath.Dir(proj.Path), "/"+filepath.Base(proj.Path), "/"+trashName); renameErr != nil {
				slog.WarnContext(ctx, "Failed to quarantine project files", "path", proj.Path, "trashPath", trashPath, "error", renameErr)
			} else {
				slog.InfoContext(ctx, "Project files quarantined successfully", "path", proj.Path, "trashPath", trashPath)
			}
		}
	}
	s.invalidateProjectCaches(projectID)

	metadata := database.JSON{"action": "destroy", "projectID": projectID, "projectName": proj.Name, "removeFiles": removeFiles, "removeVolumes": removeVolumes}
	s.logProjectEvent(ctx, event.EventTypeProjectDelete, projectID, proj.Name, user, metadata, "could not log project destroy action")

	return nil
}

func (s *ProjectService) RedeployProject(ctx context.Context, projectID string, user usertypes.Actor, options *projecttypes.DeployOptions) error {
	proj, err := s.getMutableProject(ctx, projectID)
	if err != nil {
		return err
	}

	if _, resolveProjectComposeFileErr := s.ResolveProjectComposeFile(ctx, proj); resolveProjectComposeFileErr != nil {
		return resolveProjectComposeFileErr
	}

	// Arcane's own server container must go through the system upgrade flow. A
	// failed container listing skips the guard rather than blocking the redeploy.
	if containers, listErr := s.details.ComposeContainers(ctx); listErr != nil {
		slog.WarnContext(ctx, "could not list compose containers to check self-redeploy guard; skipping guard", "error", listErr)
	} else {
		currentContainerID, currentContainerErr := cgroup.CurrentContainerID()
		projectContainers, ownerErr := s.projectContainers(ctx, *proj, containers)
		if ownerErr != nil {
			return ownerErr
		}
		if slices.ContainsFunc(projectContainers, func(c container.Summary) bool {
			return labels.ShouldDisableArcaneServerRedeploy(c.Labels, c.ID, currentContainerID, currentContainerErr)
		}) {
			return errors.New("arcane cannot redeploy itself; use the system upgrade flow (Settings -> Updates) instead")
		}
	}

	progressWriter, _ := ctx.Value(dockertypes.ProgressWriterKey{}).(io.Writer)
	if progressWriter == nil {
		progressWriter = io.Discard
	}

	credentials, cerr := s.ResolveRegistryCredentials(ctx)
	if cerr != nil {
		slog.WarnContext(ctx, "failed to resolve registry credentials for redeploy pull", "error", cerr)
	}
	if pullProjectImagesErr := s.PullProjectImages(ctx, projectID, progressWriter, user, credentials); pullProjectImagesErr != nil {
		slog.WarnContext(ctx, "failed to pull project images", "error", pullProjectImagesErr)
	}

	return s.DeployProject(ctx, projectID, user, options)
}

func (s *ProjectService) PullProjectImages(ctx context.Context, projectID string, progressWriter io.Writer, user usertypes.Actor, credentials []containerregistry.Credential) error {
	proj, err := s.getMutableProject(ctx, projectID)
	if err != nil {
		return err
	}

	compProj, _, lerr := s.loadComposeProjectForProject(ctx, proj, nil)
	if lerr != nil {
		return fmt.Errorf("failed to load compose project: %w", lerr)
	}

	defer s.eventService.BeginComposeSuppressionWindow(compProj.Name)()

	for _, imageRef := range projects.PullableImageRefs(compProj) {
		if pullErr := s.deployment.Pull(ctx, imageRef, progressWriter, user, credentials); pullErr != nil {
			return pullErr
		}
	}
	return nil
}

func (s *ProjectService) BuildProjectServices(ctx context.Context, projectID string, options projecttypes.BuildOptions, progressWriter io.Writer, user *usertypes.Actor) error {
	projectFromDb, err := s.getMutableProject(ctx, projectID)
	if err != nil {
		return err
	}

	projectModel, _, derr := s.loadComposeProjectForProject(ctx, projectFromDb, nil)
	if derr != nil {
		return fmt.Errorf("failed to load compose project in %s: %w", projectFromDb.Path, derr)
	}

	defer s.eventService.BeginComposeSuppressionWindow(projectModel.Name)()

	return s.composeCoordinator.BuildServices(ctx, projectID, projectModel, options, progressWriter, s.deployment.ImageOperations(user, nil))
}

func (s *ProjectService) RestartProject(ctx context.Context, projectID string, services []string, user usertypes.Actor) error {
	proj, err := s.getMutableProject(ctx, projectID)
	if err != nil {
		return err
	}
	if _, resolveProjectComposeFileErr := s.ResolveProjectComposeFile(ctx, proj); resolveProjectComposeFileErr != nil {
		return resolveProjectComposeFileErr
	}

	if updateProjectStatusErr := s.updateProjectStatus(ctx, projectID, ProjectStatusRestarting); updateProjectStatusErr != nil {
		return fmt.Errorf("failed to update project status to restarting: %w", updateProjectStatusErr)
	}

	compProj, _, lerr := s.loadComposeProjectForProject(ctx, proj, nil)
	if lerr != nil {
		_ = s.updateProjectStatus(ctx, projectID, ProjectStatusRunning)
		return fmt.Errorf("failed to load compose project: %w", lerr)
	}

	defer s.eventService.BeginComposeSuppressionWindow(compProj.Name)()

	if composeRestartErr := projects.ComposeRestart(ctx, compProj, services); composeRestartErr != nil {
		_ = s.updateProjectStatus(ctx, projectID, ProjectStatusRunning)
		return fmt.Errorf("failed to restart project: %w", composeRestartErr)
	}

	metadata := database.JSON{
		"action":      "restart",
		"projectID":   projectID,
		"projectName": proj.Name,
	}
	if len(services) > 0 {
		metadata["services"] = append([]string(nil), services...)
	}
	s.logProjectEvent(ctx, event.EventTypeProjectStart, projectID, proj.Name, user, metadata, "could not log project restart action")

	return s.updateProjectStatusAndCounts(ctx, projectID, ProjectStatusRunning)
}

func (s *ProjectService) updateProjectStatusAndCounts(ctx context.Context, projectID string, status ProjectStatus) error {
	services, err := s.projectServices(ctx, projectID)
	if err != nil {
		slog.ErrorContext(ctx, "loading project services failed during status update", "projectId", projectID, "error", err)
		return s.updateProjectStatus(ctx, projectID, status)
	}

	serviceCount, runningCount := listing.ServiceCounts(services)
	if updateStatusErr := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", projectID).Updates(map[string]any{
		"status":        status,
		"service_count": serviceCount,
		"running_count": runningCount,
		"updated_at":    time.Now(),
	}).Error; updateStatusErr != nil {
		return fmt.Errorf("failed to update project status and counts: %w", updateStatusErr)
	}
	return nil
}

func (s *ProjectService) updateProjectStatus(ctx context.Context, id string, status ProjectStatus) error {
	now := time.Now()
	res := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", id).Updates(map[string]any{
		"status":     status,
		"updated_at": now,
	})

	if res.Error != nil {
		return fmt.Errorf("failed to update project status: %w", res.Error)
	}

	return nil
}

func (s *ProjectService) projectContent(ctx context.Context, projectID string) (composeContent, envContent, overrideContent string, err error) {
	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return "", "", "", err
	}

	composePath, composeErr := s.ResolveProjectComposeFile(ctx, proj)
	switch {
	case composeErr == nil, errors.Is(composeErr, common.ErrProjectComposeFileNotFound):
	case errors.Is(composeErr, common.ErrProjectEnvUnreadable):
		projectsDirectory, dirErr := s.GetProjectsDirectory(ctx)
		if dirErr != nil {
			slog.DebugContext(ctx, "failed to resolve projects directory for compose identification", "projectId", proj.ID, "error", dirErr)
		}
		composePath, composeErr = projects.DetectComposeFile(ctx, projectsDirectory, proj.Path)
		if composeErr != nil && (!errors.Is(composeErr, common.ErrProjectEnvUnreadable) || composePath == "") {
			return "", "", "", fmt.Errorf("failed to identify project compose file: %w", composeErr)
		}
	default:
		return "", "", "", composeErr
	}

	composeContent, envContent, err = projects.ReadProjectFiles(ctx, proj.Path, composePath)
	if err != nil {
		return "", "", "", err
	}

	return composeContent, envContent, projects.ReadComposeOverrideContent(proj.Path), nil
}

func (s *ProjectService) GetProjectDetails(ctx context.Context, projectID string, opts projecttypes.DetailsOptions) (projecttypes.Details, error) {
	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return projecttypes.Details{}, err
	}
	projectsDir, projectsDirErr := s.GetProjectsDirectory(ctx)
	if projectsDirErr != nil {
		// Relative paths and project environment selection degrade without a
		// projects directory; keep the details response intact but log it.
		slog.WarnContext(ctx, "failed to resolve projects directory for project details", "projectId", projectID, "error", projectsDirErr)
	}

	var resp projecttypes.Details
	if mapStructErr := mapping.MapStruct(proj, &resp); mapStructErr != nil {
		return projecttypes.Details{}, fmt.Errorf("failed to map project: %w", mapStructErr)
	}

	resp.ComposeProjectName = observedComposeName(*proj)
	resp.CreatedAt = proj.CreatedAt.Format(time.RFC3339)
	resp.UpdatedAt = proj.UpdatedAt.Format(time.RFC3339)
	resp.IsArchived = proj.IsArchived
	resp.ArchivedAt = proj.ArchivedAt
	resp.HasBuildDirective = proj.BuildImageRefsJSON != nil && len(projects.ParseImageRefsJSON(*proj.BuildImageRefsJSON)) > 0
	resp.DirName = mo.PointerToOption(proj.DirName).OrEmpty()
	resp.RelativePath = listing.RelativePath(projectsDir, proj.Path)
	meta := s.ProjectMetadata(ctx, *proj, nil)
	icon := iconcatalog.Resolve(IconCatalogForContext(ctx), meta.ProjectIcon)
	resp.IconLightURL, resp.IconDarkURL = icon.IconLightURL, icon.IconDarkURL
	resp.URLs = meta.ProjectURLS
	resp.Tags, err = s.GetProjectTags(ctx, projectID)
	if err != nil {
		return projecttypes.Details{}, err
	}

	// Default counts/status from DB (will be overridden if runtime check succeeds)
	resp.ServiceCount = proj.ServiceCount
	resp.RunningCount = proj.RunningCount
	resp.Status = string(proj.Status)

	// COMPOSE_FILE selects the deployed file set (no auto-overrides). A broken
	// selection is logged but keeps the details response intact.
	composeSelection, selErr := projects.ComposeFileEnvSelection(ctx, projectsDir, proj.Path)
	if selErr != nil {
		selLogLevel := kit.Ternary(errors.Is(selErr, common.ErrProjectEnvUnreadable), slog.LevelDebug, slog.LevelWarn)
		slog.Log(ctx, selLogLevel, "failed to resolve COMPOSE_FILE selection for project details", "projectId", proj.ID, "path", proj.Path, "error", selErr)
		composeSelection = nil
	}
	resp.ComposeFiles = projectdetails.ComposeSelectionRelativePaths(proj.Path, composeSelection)
	resp.ConfigurationError = projects.CheckProjectEnvAccess(ctx, projectsDir, proj.Path)

	if opts.IncludeComposeContent {
		composeContent, _, overrideContent, contentErr := s.projectContent(ctx, proj.ID)
		if contentErr != nil {
			return projecttypes.Details{}, fmt.Errorf("failed to read project compose content: %w", contentErr)
		}
		resp.ComposeContent = composeContent
		resp.OverrideFileName, resp.OverrideContent = projectdetails.ResolveDetailsOverride(proj.Path, overrideContent, composeSelection)
	}
	if opts.IncludeEnvState {
		envState, readProjectEnvStateErr := projects.ReadProjectEnvState(proj.Path)
		if readProjectEnvStateErr != nil {
			return projecttypes.Details{}, fmt.Errorf("failed to read project env state: %w", readProjectEnvStateErr)
		}
		effectiveEnvContent, readProjectEnvStateErr := projectsync.ResolveStoredEffectiveEnvContent(envState)
		if readProjectEnvStateErr != nil {
			return projecttypes.Details{}, readProjectEnvStateErr
		}
		resp.EnvContent = effectiveEnvContent
	}

	s.enrichComposeDetails(ctx, proj, opts, &resp)

	// Refresh runtime status/counts even when callers do not request the full
	// runtime service array. DB values are only a fallback when Docker lookup
	// or compose loading fails.
	services, serr := s.projectServices(ctx, projectID)
	if serr == nil && services != nil {
		resp.ServiceCount = len(services)
		_, runningCount := listing.ServiceCounts(services)
		resp.RunningCount = runningCount
		resp.Status = listing.ProjectStatus(services)

		if opts.IncludeRuntimeServices || opts.IncludeUpdateInfo {
			resp.RuntimeServices = services
			resp.RedeployDisabled = slices.ContainsFunc(services, func(svc projecttypes.RuntimeService) bool { return svc.RedeployDisabled })
		}
	}

	if opts.IncludeUpdateInfo {
		s.enrichProjectUpdateInfo(ctx, &resp)
	}
	if !opts.IncludeRuntimeServices {
		resp.RuntimeServices = nil
	}
	if !opts.IncludeServiceConfigs {
		resp.Services = nil
	}

	return resp, nil
}

func (s *ProjectService) enrichProjectUpdateInfo(ctx context.Context, resp *projecttypes.Details) {
	if resp == nil {
		return
	}

	imageRefs := projects.ImageRefsFromComposeConfigs(resp.Services)
	if len(imageRefs) == 0 {
		imageRefs = projects.ImageRefsFromRuntimeServices(resp.RuntimeServices)
	}

	var updateInfoByRef map[string]*imagetypes.UpdateInfo
	if len(imageRefs) > 0 && s.imageService != nil {
		lookupResult, err := s.imageService.GetUpdateInfoByImageRefs(ctx, imageRefs)
		if err != nil {
			slog.WarnContext(ctx, "failed to fetch project update info", "projectId", resp.ID, "projectName", resp.Name, "error", err)
		} else {
			updateInfoByRef = lookupResult
		}
	}

	scoped := s.details.ContainerUpdateInfo(ctx, []projecttypes.Details{*resp})
	if len(resp.Services) > 0 {
		records := s.details.ServiceUpdateRecords(ctx, []string{resp.ID})
		resp.UpdateInfo = BuildConfiguredUpdateInfo(resp.ID, resp.Services, updateInfoByRef, records, projectdetails.ConfiguredRuntimeServiceUpdateInfo(resp.Services, resp.RuntimeServices, scoped))
		return
	}
	resp.UpdateInfo = buildUpdateInfoSummary(imageRefs, projectdetails.MergeProjectContainerUpdateInfo(updateInfoByRef, resp.RuntimeServices, scoped))
}

func (s *ProjectService) enrichProjectsWithUpdateInfo(
	ctx context.Context,
	projectsList []Project,
	details []projecttypes.Details,
	includeHidden bool,
	env *projectMetadataEnv,
) {
	if len(projectsList) == 0 || len(details) == 0 {
		return
	}
	if env == nil {
		env = s.newProjectMetadataEnv(ctx, projectsList)
	}

	var hiddenServicesByProjectID, hiddenRefsByProjectID map[string]map[string]bool
	if !includeHidden {
		hiddenServicesByProjectID, hiddenRefsByProjectID = projectdetails.ExcludeHiddenRuntimeServices(details)
	}

	// Bounded fan-out over compose loads; a cancelled request leaves the
	// remaining projects without configured services.
	refsByIndex := make([][]string, len(projectsList))
	servicesByIndex := make([][]types.ServiceConfig, len(projectsList))
	var g errgroup.Group
	g.SetLimit(maxConcurrentComposeReads)
	for i, proj := range projectsList {
		g.Go(func() error {
			if ctx.Err() != nil {
				return nil
			}
			if len(details[i].RuntimeServices) > 0 && details[i].RuntimeServices[0].Status != projecttypes.ServiceNotCreated {
				refsByIndex[i] = projects.ImageRefsFromRuntimeServices(details[i].RuntimeServices)
				return nil
			}
			composeProject, err := s.getCachedComposeProject(ctx, &proj, env)
			if err != nil {
				slog.WarnContext(ctx, "failed to resolve project services for update summary", "projectId", proj.ID, "projectName", proj.Name, "error", err)
				refsByIndex[i] = slices.DeleteFunc(projects.ParseImageRefsJSON(proj.ImageRefsJSON), func(ref string) bool { return hiddenRefsByProjectID[proj.ID][ref] })
				return nil
			}
			services := make([]types.ServiceConfig, 0, len(composeProject.Services))
			for _, service := range composeProject.Services {
				hidden, _ := kit.ParseBool(service.Labels[libarcane.HiddenResourceLabel])
				if includeHidden || (!hidden && !hiddenServicesByProjectID[proj.ID][service.Name]) {
					services = append(services, service)
				}
			}
			refsByIndex[i], servicesByIndex[i] = projects.ImageRefsFromComposeConfigs(services), services
			return nil
		})
	}
	_ = g.Wait()

	projectIDs := make([]string, len(projectsList))
	imageRefsByProjectID := make(map[string][]string, len(projectsList))
	servicesByProjectID := make(map[string][]types.ServiceConfig, len(projectsList))
	for i, proj := range projectsList {
		projectIDs[i] = proj.ID
		imageRefsByProjectID[proj.ID], servicesByProjectID[proj.ID] = refsByIndex[i], servicesByIndex[i]
	}
	allImageRefs := slices.Concat(refsByIndex...)

	var updateInfoByRef map[string]*imagetypes.UpdateInfo
	if len(allImageRefs) > 0 && s.imageService != nil {
		lookupResult, err := s.imageService.GetUpdateInfoByImageRefs(ctx, allImageRefs)
		if err != nil {
			slog.WarnContext(ctx, "failed to fetch project list update info", "error", err)
		} else {
			updateInfoByRef = lookupResult
		}
	}

	recordsByProjectID := make(map[string][]imageupdate.ImageUpdateRecord)
	for _, record := range s.details.ServiceUpdateRecords(ctx, projectIDs) {
		recordsByProjectID[record.ProjectID] = append(recordsByProjectID[record.ProjectID], record)
	}
	scoped := s.details.ContainerUpdateInfo(ctx, details)
	for i := range details {
		if services := servicesByProjectID[details[i].ID]; services != nil {
			details[i].UpdateInfo = BuildConfiguredUpdateInfo(
				details[i].ID,
				services,
				updateInfoByRef,
				recordsByProjectID[details[i].ID],
				projectdetails.ConfiguredRuntimeServiceUpdateInfo(
					services,
					details[i].RuntimeServices,
					scoped,
				),
			)
			continue
		}
		refs := imageRefsByProjectID[details[i].ID]
		if len(refs) == 0 {
			refs = projects.ImageRefsFromRuntimeServices(details[i].RuntimeServices)
		}
		details[i].UpdateInfo = buildUpdateInfoSummary(refs, projectdetails.MergeProjectContainerUpdateInfo(updateInfoByRef, details[i].RuntimeServices, scoped))
	}
}

// BuildConfiguredUpdateInfo matches checks to current services and aggregates their results.
func BuildConfiguredUpdateInfo(
	projectID string,
	services []types.ServiceConfig,
	byRef map[string]*imagetypes.UpdateInfo,
	records []imageupdate.ImageUpdateRecord,
	runtimeUpdates ...map[string]*imagetypes.UpdateInfo,
) *projecttypes.UpdateInfo {
	checks := make(map[string]*imageupdate.ImageUpdateRecord)
	for i := range records {
		if records[i].ProjectID == projectID {
			checks[records[i].ServiceName] = &records[i]
		}
	}
	serviceUpdates := make(map[string]projecttypes.ServiceUpdateInfo, len(services))
	selected := make(map[string]*imagetypes.UpdateInfo)
	runtime := make([]projecttypes.RuntimeService, 0, len(services))
	unknownRefs := make(map[string]bool)
	for _, service := range services {
		imageRef := strings.TrimSpace(service.Image)
		if imageRef == "" {
			continue
		}
		var info *imagetypes.UpdateInfo
		record := checks[service.Name]
		policy, policyErr := tagpolicy.Resolve(imageRef, updater.DefaultLabelPolicy().TagPolicy(service.Labels))
		if record != nil && record.PolicyKey == imageref.UpdatePolicyKey(imageRef, service.Labels) {
			info = record.UpdateInfo()
		} else if policyErr == nil && policy.Strategy == "digest" && !imageref.IsUpdateCheckDisabled(service.Labels) {
			info = byRef[imageRef]
		}

		if len(runtimeUpdates) > 0 && runtimeUpdates[0][service.Name] != nil {
			// A newer clean runtime check clears a stale update preview (#4306);
			// digest checks must match the configured reference to count.
			runtime := runtimeUpdates[0][service.Name]
			attributed := policyErr == nil && policy.Strategy != "digest"
			if stored := byRef[imageRef]; policyErr == nil && !attributed && stored != nil && !stored.HasUpdate && stored.Error == "" && stored.CheckTime.Equal(runtime.CheckTime) {
				attributed = stored.CurrentDigest == runtime.CurrentDigest && stored.LatestDigest == runtime.LatestDigest
			}
			if attributed && info != nil && info.HasUpdate && !runtime.HasUpdate && runtime.Error == "" && !runtime.CheckTime.Before(info.CheckTime) {
				info = nil
			}
			info = projectdetails.MergeProjectContainerUpdateInfo(
				nil,
				[]projecttypes.RuntimeService{
					{
						ContainerID: "preview",
						Image:       imageRef,
					},
					{
						ContainerID: "runtime",
						Image:       imageRef,
					},
				},
				map[string]*imagetypes.UpdateInfo{
					"preview": info,
					"runtime": runtimeUpdates[0][service.Name],
				},
			)[imageRef]
		}
		serviceUpdates[service.Name] = projecttypes.ServiceUpdateInfo{ImageRef: imageRef, UpdateInfo: info}
		selected[service.Name] = info
		runtime = append(runtime, projecttypes.RuntimeService{ContainerID: service.Name, Image: imageRef})
		if info == nil {
			unknownRefs[imageRef] = true
		}
	}
	merged := projectdetails.MergeProjectContainerUpdateInfo(nil, runtime, selected)
	for imageRef := range unknownRefs {
		if info := merged[imageRef]; info == nil || !info.HasUpdate {
			delete(merged, imageRef)
		}
	}
	summary := buildUpdateInfoSummary(projects.ImageRefsFromComposeConfigs(services), merged)
	summary.ServiceUpdates = serviceUpdates
	return summary
}
