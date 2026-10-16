// Source: getarcaneapp/arcane/backend/internal/project/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite-only local persistence.

package project

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/deployment"
	projectdetails "github.com/florianfinn/docklet-next/backend/internal/project/children/details"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/listing"
	projectsync "github.com/florianfinn/docklet-next/backend/internal/project/children/sync"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/update"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/workspace"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/client"
	"github.com/samber/mo"
	dockertypes "go.getarcane.app/docker/types"
	kit "go.getarcane.app/kit/pkg"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxConcurrentComposeReads = 8
	// inferredServiceCountBatchSize bounds the number of placeholders in each update.
	inferredServiceCountBatchSize = 200
)

var (
	composeStopProjectServices = projects.ComposeStop
	composeUpProjectServices   = projects.ComposeUp
)

type ProjectService struct {
	composeCoordinator          projecttypes.ComposeCoordinator
	db                          *database.DB
	settingsService             *settings.SettingsService
	eventService                *event.EventService
	imageService                *image.ImageService
	dockerService               *dockerInternal.DockerClientService
	workspace                   *workspace.Service
	details                     *projectdetails.Service
	listing                     *listing.Service
	deployment                  *deployment.Service
	updates                     *update.Service
	containerRegistryService    *registry.ContainerRegistryService
	config                      *config.Config
	RegistryCredentialsProvider func(context.Context) ([]containerregistry.Credential, error)

	// syncMu serializes SyncProjectsFromFileSystem: its discovery walk and its
	// cleanup pass must not interleave with another run's.
	syncMu sync.Mutex

	composeNames  composeNameCache
	parsedCompose projecttypes.ComposeCache[*types.Project]
	// metaCache holds per-project icon/URL metadata, keyed by project ID.
	// Entries are validated by compose/include/env file mtimes rather than a TTL.
	metaCache projecttypes.ComposeCache[projects.ArcaneComposeMetadata]
}

func (s *ProjectService) newProjectMetadataEnv(ctx context.Context, projectsList []Project) *projectMetadataEnv {
	projectsDirectory, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		slog.WarnContext(ctx, "failed to resolve projects directory for compose selection", "error", err)
	}
	env := &projectMetadataEnv{
		projectsDirectory: projectsDirectory,
		autoInjectEnv:     s.settingsService.GetBoolSetting(ctx, "autoInjectEnv", false),
		settings:          s.settingsService.GetSettingsOrDefaults(ctx),
		composeFiles:      make(map[string]string, len(projectsList)),
	}
	return env
}

func NewProjectService(
	db *database.DB,
	settingsService *settings.SettingsService,
	eventService *event.EventService,
	imageService *image.ImageService,
	dockerService *dockerInternal.DockerClientService,
	buildService builder,
	containerRegistryService *registry.ContainerRegistryService,
	cfg *config.Config,
	kvService *kv.KVService,
	registryCredentialsProvider func(context.Context) ([]containerregistry.Credential, error),
) *ProjectService {
	s := &ProjectService{
		RegistryCredentialsProvider: registryCredentialsProvider,
		composeCoordinator:          projects.NewCoordinator(projecttypes.ComposeCommands{Stop: composeStopProjectServices, Up: composeUpProjectServices, Create: projects.ComposeCreate}),
		db:                          db,
		settingsService:             settingsService,
		eventService:                eventService,
		imageService:                imageService,
		dockerService:               dockerService,
		containerRegistryService:    containerRegistryService,
		config:                      cfg,
		parsedCompose:               projects.NewParsedComposeCache(),
		metaCache:                   projects.NewComposeCache[projects.ArcaneComposeMetadata](1024, nil),
	}
	s.initChildren(kvService, buildService)
	return s
}

// initChildren builds the feature services from the parent's
// dependencies and callbacks.
func (s *ProjectService) initChildren(kvService *kv.KVService, buildService builder) {
	s.workspace = workspace.New(s.config)
	s.details = projectdetails.New(s.db, s.dockerService, s.imageService, s.settingsService)
	s.listing = listing.New(
		s.details.ComposeContainers,
		s.imageService,
		func(imageRefs []string, services []projecttypes.RuntimeService, scoped map[string]*imagetypes.UpdateInfo) *projecttypes.UpdateInfo {
			return buildUpdateInfoSummary(imageRefs, projectdetails.MergeProjectContainerUpdateInfo(nil, services, scoped))
		},
	)
	s.deployment = deployment.New(s.settingsService, s.imageService, s.dockerService, buildService)
	// Rename recovery compares the stored name and path against its journal and
	// rolls the row back to its pre-rename identity.
	renameState := func(ctx context.Context, projectID string) (string, string, bool, error) {
		proj, found, err := s.FindProjectByID(ctx, projectID)
		if err != nil || !found {
			return "", "", found, err
		}
		return proj.Name, proj.Path, true, nil
	}
	restoreRenamed := func(ctx context.Context, journal *projecttypes.RenameJournal) error {
		return s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", journal.ProjectID).Updates(map[string]any{
			"name": journal.OldName, "path": journal.OldPath, "dir_name": journal.OldDirName,
		}).Error
	}
	s.updates = update.New(kvService, s.dockerService, s.containerRegistryService, s.ResolveRegistryCredentials, renameState, restoreRenamed)
}

// RecoverProjectRenameJournals replays renames interrupted by a restart.
func (s *ProjectService) RecoverProjectRenameJournals(ctx context.Context) error {
	return s.updates.RecoverAll(ctx)
}

func (s *ProjectService) ResolveRegistryCredentials(ctx context.Context) ([]containerregistry.Credential, error) {
	if s == nil || s.RegistryCredentialsProvider == nil {
		return nil, nil
	}

	credentials, err := s.RegistryCredentialsProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("get enabled registry credentials: %w", err)
	}

	return credentials, nil
}

func (s *ProjectService) GetProjectsDirectory(ctx context.Context) (string, error) {
	projectsDir, err := projects.GetProjectsDirectory(ctx, s.settingsService.GetStringSetting(ctx, "projectsDirectory", "/app/data/projects"))
	if err != nil {
		return "", err
	}

	return filepath.Clean(projectsDir), nil
}

func (s *ProjectService) getMutableProject(ctx context.Context, projectID string) (*Project, error) {
	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if proj != nil && proj.IsArchived {
		return nil, common.Classify(common.ErrProjectArchived, errors.New("project is archived and must be unarchived before this action"))
	}
	return proj, nil
}

func (s *ProjectService) logProjectEvent(ctx context.Context, eventType event.EventType, projectID, projectName string, user usertypes.Actor, metadata database.JSON, action string) {
	if s.eventService == nil {
		return
	}
	if logErr := s.eventService.LogProjectEvent(ctx, eventType, projectID, projectName, user.ID, user.Username, "0", metadata); logErr != nil {
		slog.ErrorContext(ctx, "could not log project event", "action", action, "error", logErr)
	}
}

func (s *ProjectService) GetProjectRelativePath(ctx context.Context, projectPath string) string {
	projectsDir, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		return ""
	}

	return listing.RelativePath(projectsDir, projectPath)
}

func (s *ProjectService) GetProjectFromDatabaseByID(ctx context.Context, id string) (*Project, error) {
	projectModel, found, err := s.FindProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("request canceled or timed out")
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if !found {
		return nil, errors.New("project not found")
	}
	return projectModel, nil
}

// FindProjectByID reports a missing project as found=false instead of an error.
func (s *ProjectService) FindProjectByID(ctx context.Context, projectID string) (*Project, bool, error) {
	var project Project
	if err := s.db.WithContext(ctx).Where("id = ?", projectID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &project, true, nil
}

func (s *ProjectService) GetProjectByComposeName(ctx context.Context, name string) (*Project, error) {
	if name == "" {
		return nil, errors.New("project name is empty")
	}
	normalized := projects.NormalizeProjectName(name)

	var proj Project
	err := s.db.WithContext(ctx).Where("name = ? OR name = ?", name, normalized).First(&proj).Error
	if err == nil {
		s.composeNames.put(normalized, proj.ID)
		return &proj, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to get project by name: %w", err)
	}

	if cachedProject, found, cacheErr := s.lookupProjectByCachedComposeName(ctx, normalized); cacheErr != nil {
		return nil, cacheErr
	} else if found {
		return cachedProject, nil
	}

	var projectModels []Project
	if listErr := s.db.WithContext(ctx).Select("id", "name").Find(&projectModels).Error; listErr != nil {
		return nil, fmt.Errorf("failed to list projects by compose name: %w", listErr)
	}
	byName := make(map[string]string, len(projectModels))
	for _, projectModel := range projectModels {
		if normalizedName := projects.NormalizeProjectName(projectModel.Name); normalizedName != "" {
			if _, exists := byName[normalizedName]; !exists {
				byName[normalizedName] = projectModel.ID
			}
		}
	}
	s.composeNames.replace(byName)

	if cachedProject, found, cacheErr := s.lookupProjectByCachedComposeName(ctx, normalized); cacheErr != nil {
		return nil, cacheErr
	} else if found {
		return cachedProject, nil
	}

	return nil, common.Classify(common.ErrNotFound, fmt.Errorf("project not found: %s", name))
}

// ComposeServiceImage returns the effective image of serviceName in the
// Arcane project deployed as composeName.
func (s *ProjectService) ComposeServiceImage(ctx context.Context, composeName, serviceName string) (projectID, imageRef string, err error) {
	proj, err := s.GetProjectByComposeName(ctx, composeName)
	if err != nil {
		return "", "", err
	}
	effective, _, err := s.loadComposeProjectForProject(ctx, proj, nil, serviceName)
	if err != nil {
		return "", "", fmt.Errorf("load project %s: %w", proj.Name, err)
	}
	service, ok := effective.Services[serviceName]
	if !ok {
		return "", "", fmt.Errorf("service %s is not active in project %s", serviceName, proj.Name)
	}
	return proj.ID, service.Image, nil
}

// EnsureProjectPathUnderRoot validates that the project's path is a safe subdirectory of the configured projects root.
// If not, it normalizes the path to `<projectsRoot>/<dirName or sanitized project name>`. When persist=true, it saves
// the updated project path to the database.
func (s *ProjectService) EnsureProjectPathUnderRoot(ctx context.Context, proj *Project, persist bool) error {
	projectsDirectory, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		return fmt.Errorf("failed to get projects directory: %w", err)
	}

	rootAbs, _ := filepath.Abs(projectsDirectory)
	rootAbs = filepath.Clean(rootAbs)

	projPathAbs := proj.Path
	if abs, aerr := filepath.Abs(proj.Path); aerr == nil {
		projPathAbs = filepath.Clean(abs)
	}

	if projects.IsSafeSubdirectory(rootAbs, projPathAbs) {
		return nil
	}

	// Attempt to repair using known directory name or sanitized project name
	dirName := mo.PointerToOption(proj.DirName).OrEmpty()
	if strings.TrimSpace(dirName) == "" {
		dirName = projects.SanitizeProjectName(proj.Name)
	}
	candidate := filepath.Join(projectsDirectory, dirName)

	slog.WarnContext(ctx, "Normalizing project path to projects root", "projectId", proj.ID, "oldPath", proj.Path, "newPath", candidate, "root", projectsDirectory)
	proj.Path = filepath.Clean(candidate)

	if persist {
		if saveErr := s.db.WithContext(ctx).Save(proj).Error; saveErr != nil {
			slog.WarnContext(ctx, "failed to persist normalized project path", "error", saveErr)
		}
	}
	return nil
}

func (s *ProjectService) projectPathMapper(ctx context.Context) *projects.PathMapper {
	var dockerClient *client.Client
	if s.dockerService != nil {
		dockerClient, _ = s.dockerService.GetClient(ctx)
	}
	return projects.NewPathMapperForConfiguredDirectory(
		ctx,
		s.settingsService.GetStringSetting(ctx, "projectsDirectory", "/app/data/projects"),
		"/app/data/projects",
		dockerClient,
	)
}

func (s *ProjectService) invalidateProjectCaches(projectID string) {
	if s.parsedCompose != nil {
		s.parsedCompose.Invalidate(projectID)
	}
	if s.metaCache != nil {
		s.metaCache.Invalidate(projectID)
	}
}

func (s *ProjectService) lookupProjectByCachedComposeName(ctx context.Context, normalizedName string) (*Project, bool, error) {
	projectID, ok := s.composeNames.projectID(normalizedName).Get()
	if !ok {
		return nil, false, nil
	}

	var projectModel Project
	if err := s.db.WithContext(ctx).Where("id = ?", projectID).First(&projectModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.composeNames.invalidate(normalizedName)
			return nil, false, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, false, fmt.Errorf("request canceled or timed out: %w", err)
		}
		return nil, false, fmt.Errorf("failed to get project by cached compose name: %w", err)
	}
	if projects.NormalizeProjectName(projectModel.Name) != normalizedName {
		s.composeNames.invalidate(normalizedName)
		return nil, false, nil
	}

	return &projectModel, true, nil
}

// ResolveProjectComposeFile returns the base compose file for a project. The
// precedence mirrors `docker compose`: COMPOSE_FILE in the merged environment
// (the project .env) wins, then standard detection.
func (s *ProjectService) ResolveProjectComposeFile(ctx context.Context, proj *Project) (string, error) {
	return s.resolveProjectComposeFile(ctx, proj, nil)
}

// resolveProjectComposeFile is ResolveProjectComposeFile with the
// request-scoped inputs supplied by env; a nil env resolves them per call.
func (s *ProjectService) resolveProjectComposeFile(ctx context.Context, proj *Project, env *projectMetadataEnv) (string, error) {
	if proj == nil {
		return "", errors.New("project is nil")
	}
	return env.composeFile(proj.ID, func() (string, error) {
		projectsDirectory := ""
		switch {
		case env != nil:
			projectsDirectory = env.projectsDirectory
		case s.settingsService != nil:
			var dirErr error
			projectsDirectory, dirErr = s.GetProjectsDirectory(ctx)
			if dirErr != nil {
				// The project root is unavailable for an empty projects directory;
				// keep resolution working but surface the misconfiguration.
				slog.WarnContext(ctx, "failed to resolve projects directory for compose selection", "projectId", proj.ID, "error", dirErr)
			}
		}
		if files, selErr := projects.ComposeFileEnvSelection(ctx, projectsDirectory, proj.Path); selErr != nil {
			return "", selErr
		} else if len(files) > 0 {
			return files[0], nil
		}

		composeFile, err := projects.DetectComposeFile(ctx, projectsDirectory, proj.Path)
		if err != nil {
			if errors.Is(err, common.ErrProjectEnvUnreadable) {
				return "", err
			}
			return "", common.Classify(common.ErrProjectComposeFileNotFound, fmt.Errorf("Project compose file not found: %w", err))
		}
		return composeFile, nil
	})
}

// loadComposeProjectForProject loads the executable compose model for
// proj. prepare is optional and runs before host path translation; deployment
// paths use it to create missing bind directories, read paths pass nil.
func (s *ProjectService) loadComposeProjectForProject(ctx context.Context, proj *Project, prepare projects.PrepareProjectFunc, services ...string) (*types.Project, string, error) {
	composeFileFullPath, err := s.ResolveProjectComposeFile(ctx, proj)
	if err != nil {
		return nil, "", err
	}

	cfg := s.settingsService.GetSettingsOrDefaults(ctx)
	projectsDirectory, dirErr := projects.GetProjectsDirectory(ctx, cfg.ProjectsDirectory.Value)
	if dirErr != nil {
		slog.WarnContext(ctx, "unable to determine projects directory; using default", "error", dirErr)
		projectsDirectory = "/app/data/projects"
	}

	pathMapper := s.projectPathMapper(ctx)

	autoInjectEnv := kit.ParseOrDefault(cfg.AutoInjectEnv.Value, false, strconv.ParseBool)
	projectName := projects.NormalizeProjectName(proj.Name)
	composeProject, loadErr := projects.LoadComposeProject(ctx, composeFileFullPath, projectName, projectsDirectory, autoInjectEnv, pathMapper, nil, nil, false, nil, services, prepare)
	if loadErr != nil {
		return nil, "", loadErr
	}

	return composeProject, composeFileFullPath, nil
}

func (s *ProjectService) getCachedComposeProject(ctx context.Context, proj *Project, env *projectMetadataEnv) (*types.Project, error) {
	if proj == nil {
		return nil, errors.New("project is nil")
	}
	var cfg *settings.Settings
	if env != nil {
		cfg = env.settings
	}
	if cfg == nil {
		cfg = s.settingsService.GetSettingsOrDefaults(ctx)
	}
	composePath, err := s.resolveProjectComposeFile(ctx, proj, env)
	if err != nil {
		return nil, err
	}
	autoInjectEnv := kit.ParseOrDefault(cfg.AutoInjectEnv.Value, false, strconv.ParseBool)
	projectsDirectory, dirErr := projects.GetProjectsDirectory(ctx, cfg.ProjectsDirectory.Value)
	if dirErr != nil {
		slog.WarnContext(ctx, "unable to determine projects directory; using default", "error", dirErr)
		projectsDirectory = "/app/data/projects"
	}
	return projects.LoadCachedComposeProject(ctx, s.parsedCompose, proj.ID, proj.Path, composePath, projects.NormalizeProjectName(proj.Name), projectsDirectory, autoInjectEnv, s.projectPathMapper(ctx))
}

func (s *ProjectService) refreshComposeProjectName(ctx context.Context, proj *Project) {
	if proj == nil {
		return
	}

	cfg := s.settingsService.GetSettingsOrDefaults(ctx)
	projectsDirectory, err := projects.GetProjectsDirectory(ctx, cfg.ProjectsDirectory.Value)
	var meta projecttypes.ComposeIdentity
	if err == nil {
		dirName := cmp.Or(mo.PointerToOption(proj.DirName).OrEmpty(), proj.Name)
		autoInjectEnv := kit.ParseOrDefault(cfg.AutoInjectEnv.Value, false, strconv.ParseBool)
		meta, err = projectsync.LoadComposeMetadata(ctx, proj.Path, dirName, projectsDirectory, autoInjectEnv, s.projectPathMapper(ctx))
	}
	if err != nil {
		if errors.Is(err, common.ErrProjectEnvUnreadable) {
			slog.DebugContext(ctx, "skipped compose project name refresh; project env is unreadable", "projectId", proj.ID, "path", proj.Path, "error", err)
			return
		}
		slog.WarnContext(ctx, "failed to refresh compose project name", "projectId", proj.ID, "path", proj.Path, "error", err)
		return
	}

	updates := map[string]any{}
	shouldUpdateName := meta.ExplicitProjectName || projects.NormalizeProjectName(proj.Name) != proj.Name
	if shouldUpdateName && meta.ResolvedProjectName != "" && proj.Name != meta.ResolvedProjectName {
		updates["name"] = meta.ResolvedProjectName
	}
	if mo.PointerToOption(proj.ComposeProjectName) != mo.PointerToOption(meta.ComposeProjectName) {
		updates["compose_project_name"] = meta.ComposeProjectName
	}
	if len(updates) == 0 {
		return
	}

	updates["updated_at"] = time.Now()
	if persistComposeNameErr := s.db.WithContext(ctx).
		Model(&Project{}).
		Where("id = ?", proj.ID).
		Updates(updates).Error; persistComposeNameErr != nil {
		slog.WarnContext(ctx, "failed to persist refreshed compose project name", "projectId", proj.ID, "error", persistComposeNameErr)
		return
	}

	if name, ok := updates["name"].(string); ok {
		proj.Name = name
	}
	if _, ok := updates["compose_project_name"]; ok {
		proj.ComposeProjectName = meta.ComposeProjectName
	}
}

func (s *ProjectService) UpdateProjectServices(ctx context.Context, projectID string, servicesToUpdate []string, user usertypes.Actor, discoverTags bool) error {
	proj, err := s.getMutableProject(ctx, projectID)
	if err != nil {
		return err
	}
	if discoverTags {
		effective, _, loadErr := s.loadComposeProjectForProject(ctx, proj, nil, servicesToUpdate...)
		if loadErr != nil {
			return fmt.Errorf("load project for service image checks: %w", loadErr)
		}
		changes, changesErr := s.updates.ImageChanges(ctx, effective)
		if changesErr != nil {
			return changesErr
		}
		if len(changes) > 0 {
			if _, saveErr := s.SaveProjectServiceImages(ctx, projectID, changes); saveErr != nil {
				return saveErr
			}
		}
	}
	previousStatus := proj.Status

	// 1. Load project
	prepare := deployment.PrepareProjectBindDirectories(proj.Path)
	compProj, _, err := s.loadComposeProjectForProject(ctx, proj, prepare, servicesToUpdate...)
	if err != nil {
		return fmt.Errorf("failed to load compose project: %w", err)
	}
	dependents, stoppedDependents := s.deployment.NamespaceDependents(ctx, compProj, servicesToUpdate)
	if len(dependents)+len(stoppedDependents) > 0 {
		slog.InfoContext(ctx, "recreating namespace dependents with updated services", "projectId", projectID, "services", servicesToUpdate, "dependents", dependents, "stoppedDependents", stoppedDependents)
		if compProj, _, err = s.loadComposeProjectForProject(ctx, proj, prepare, slices.Concat(servicesToUpdate, dependents, stoppedDependents)...); err != nil {
			return fmt.Errorf("failed to load compose project with dependents: %w", err)
		}
	}

	defer s.eventService.BeginComposeSuppressionWindow(compProj.Name)()

	// 2. Set status to deploying/restarting
	if updateProjectStatusErr := s.updateProjectStatus(ctx, projectID, ProjectStatusDeploying); updateProjectStatusErr != nil {
		return updateProjectStatusErr
	}

	credentials, err := s.ResolveRegistryCredentials(ctx)
	if err != nil {
		if statusErr := s.updateProjectStatus(ctx, projectID, previousStatus); statusErr != nil {
			slog.ErrorContext(ctx, "UpdateProjectServices: failed to restore project status after credential lookup failure", "projectId", projectID, "error", statusErr)
		}
		return fmt.Errorf("resolve registry credentials: %w", err)
	}

	authConfigs, authErr := s.containerRegistryService.GetAllRegistryAuthConfigs(ctx)
	if authErr != nil {
		slog.WarnContext(ctx, "failed to load registry auth for compose pulls", "error", authErr)
	}
	progressWriter, _ := ctx.Value(dockertypes.ProgressWriterKey{}).(io.Writer)
	if updateServicesErr := s.composeCoordinator.UpdateServices(ctx, projecttypes.ComposeServiceUpdate{
		Project: compProj, Services: servicesToUpdate, Dependents: dependents, StoppedDependents: stoppedDependents,
		Images: s.deployment.ImageOperations(&user, credentials), Progress: progressWriter,
		AuthConfigs: authConfigs, WaitTimeout: timeouts.GetDuration(s.settingsService.GetSettingsConfig().DeployWaitTimeout.AsInt(), timeouts.DefaultDeployWait),
		RestoreBeforeMutation: func(ctx context.Context) {
			if statusErr := s.updateProjectStatus(ctx, projectID, previousStatus); statusErr != nil {
				slog.ErrorContext(ctx, "failed to restore project status before service update", "projectId", projectID, "error", statusErr)
			}
		},
		Recover: func(ctx context.Context) {
			services, servicesErr := s.projectServices(ctx, projectID)
			if servicesErr != nil {
				slog.WarnContext(ctx, "failed to inspect project services after deploy failure", "projectId", projectID, "error", servicesErr)
			} else {
				serviceCount, runningCount := listing.ServiceCounts(services)
				updateErr := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", projectID).Updates(map[string]any{
					"status":        ProjectStatus(listing.ProjectStatus(services)),
					"service_count": serviceCount,
					"running_count": runningCount,
					"updated_at":    time.Now(),
				}).Error
				if updateErr == nil {
					return
				}
				slog.WarnContext(ctx, "failed to restore project status after deploy failure", "projectId", projectID, "error", updateErr)
			}
			if updateErr := s.updateProjectStatus(ctx, projectID, ProjectStatusStopped); updateErr != nil {
				slog.WarnContext(ctx, "failed to set stopped status after deploy failure", "projectId", projectID, "error", updateErr)
			}
		},
	}); updateServicesErr != nil {
		return updateServicesErr
	}

	// 6. Finalize status
	if updateProjectStatusandCountsErr := s.updateProjectStatusAndCounts(ctx, projectID, ProjectStatusRunning); updateProjectStatusandCountsErr != nil {
		return updateProjectStatusandCountsErr
	}

	metadata := database.JSON{
		"action":      "update_services",
		"projectID":   projectID,
		"projectName": proj.Name,
		"services":    append([]string(nil), servicesToUpdate...),
	}
	if len(dependents) > 0 {
		metadata["dependents"] = dependents
	}
	if len(stoppedDependents) > 0 {
		metadata["stoppedDependents"] = stoppedDependents
	}
	s.logProjectEvent(ctx, event.EventTypeProjectUpdate, projectID, proj.Name, user, metadata, "could not log project service update action")

	return nil
}

func (s *ProjectService) ArchiveProject(ctx context.Context, projectID string, user usertypes.Actor) error {
	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return err
	}
	if proj.IsArchived {
		return nil
	}

	// Gate on live Docker state, not the persisted status row, which can go
	// stale when containers are stopped outside an Arcane project action.
	// A project without a compose file cannot have managed containers running
	// and is a prime archive candidate, so it is allowed through.
	services, servicesErr := s.projectServices(ctx, projectID)
	switch {
	case servicesErr != nil && !errors.Is(servicesErr, common.ErrProjectComposeFileNotFound):
		return fmt.Errorf("cannot verify project is stopped before archiving: %w", servicesErr)
	case servicesErr == nil:
		if _, running := listing.ServiceCounts(services); running > 0 {
			return common.Classify(common.ErrProjectMustBeStopped, errors.New("project must be stopped before archiving"))
		}
	}

	now := time.Now()
	if archiveProjectErr := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", projectID).Updates(map[string]any{
		"is_archived": true,
		"archived_at": now,
	}).Error; archiveProjectErr != nil {
		return fmt.Errorf("failed to archive project: %w", archiveProjectErr)
	}

	metadata := database.JSON{"action": "archived", "projectID": projectID, "projectName": proj.Name}
	s.logProjectEvent(ctx, event.EventTypeProjectUpdate, projectID, proj.Name, user, metadata, "could not log project archive action")

	return nil
}

func (s *ProjectService) UnarchiveProject(ctx context.Context, projectID string, user usertypes.Actor) error {
	proj, err := s.GetProjectFromDatabaseByID(ctx, projectID)
	if err != nil {
		return err
	}
	if !proj.IsArchived {
		return nil
	}

	if unarchiveProjectErr := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", projectID).Updates(map[string]any{
		"is_archived": false,
		"archived_at": gorm.Expr("NULL"),
	}).Error; unarchiveProjectErr != nil {
		return fmt.Errorf("failed to unarchive project: %w", unarchiveProjectErr)
	}

	metadata := database.JSON{"action": "unarchived", "projectID": projectID, "projectName": proj.Name}
	s.logProjectEvent(ctx, event.EventTypeProjectUpdate, projectID, proj.Name, user, metadata, "could not log project unarchive action")

	return nil
}

func (s *ProjectService) DeployProject(ctx context.Context, projectID string, user usertypes.Actor, options *projecttypes.DeployOptions) error {
	projectFromDb, err := s.getMutableProject(ctx, projectID)
	switch {
	case errors.Is(err, common.ErrProjectArchived):
		return err
	case err != nil:
		return fmt.Errorf("failed to get project: %w", err)
	}
	if _, resolveProjectComposeFileErr := s.ResolveProjectComposeFile(ctx, projectFromDb); resolveProjectComposeFileErr != nil {
		return resolveProjectComposeFileErr
	}

	if updateProjectStatusErr := s.updateProjectStatus(ctx, projectID, ProjectStatusDeploying); updateProjectStatusErr != nil {
		return fmt.Errorf("failed to update project status to deploying: %w", updateProjectStatusErr)
	}
	var closeSuppression func()
	defer func() {
		if closeSuppression != nil {
			closeSuppression()
		}
	}()

	authConfigs, authErr := s.containerRegistryService.GetAllRegistryAuthConfigs(ctx)
	if authErr != nil {
		slog.WarnContext(ctx, "failed to load registry auth for compose pulls", "error", authErr)
	}
	progressWriter, _ := ctx.Value(dockertypes.ProgressWriterKey{}).(io.Writer)
	projectModel, err := s.composeCoordinator.Deploy(ctx, projecttypes.ComposeDeployment{
		ProjectID: projectID, ProjectPath: projectFromDb.Path, Options: options,
		DefaultPullPolicy: s.settingsService.GetStringSetting(ctx, "defaultDeployPullPolicy", "missing"),
		WaitTimeout:       timeouts.GetDuration(s.settingsService.GetSettingsConfig().DeployWaitTimeout.AsInt(), timeouts.DefaultDeployWait),
		AuthConfigs:       authConfigs,
		Progress:          progressWriter,
		Load: func(ctx context.Context) (*types.Project, error) {
			model, _, loadComposeProjectForProjectErr := s.loadComposeProjectForProject(ctx, projectFromDb, deployment.PrepareProjectBindDirectories(projectFromDb.Path))
			if loadComposeProjectForProjectErr == nil {
				closeSuppression = s.eventService.BeginComposeSuppressionWindow(model.Name)
			}
			return model, loadComposeProjectForProjectErr
		},
		ResolveImages: func(ctx context.Context) (projecttypes.ComposeImageOperations, error) {
			credentials, resolveRegistryCredentialsErr := s.ResolveRegistryCredentials(ctx)
			operations := s.deployment.ImageOperations(&user, credentials)
			// Deployment pulls are recorded as system actions; builds retain the requesting actor.
			operations.Pull = s.deployment.ImageOperations(nil, credentials).Pull
			return operations, resolveRegistryCredentialsErr
		},
		Recover: func(ctx context.Context) {
			services, servicesErr := s.projectServices(ctx, projectID)
			if servicesErr != nil {
				slog.WarnContext(ctx, "failed to inspect project services after deploy failure", "projectId", projectID, "error", servicesErr)
			} else {
				serviceCount, runningCount := listing.ServiceCounts(services)
				updateErr := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", projectID).Updates(map[string]any{
					"status":        ProjectStatus(listing.ProjectStatus(services)),
					"service_count": serviceCount,
					"running_count": runningCount,
					"updated_at":    time.Now(),
				}).Error
				if updateErr == nil {
					return
				}
				slog.WarnContext(ctx, "failed to restore project status after deploy failure", "projectId", projectID, "error", updateErr)
			}
			if updateErr := s.updateProjectStatus(ctx, projectID, ProjectStatusStopped); updateErr != nil {
				slog.WarnContext(ctx, "failed to set stopped status after deploy failure", "projectId", projectID, "error", updateErr)
			}
		},
	})
	if err != nil {
		return err
	}

	metadata := database.JSON{"action": "deploy", "projectID": projectID, "projectName": projectModel.Name}
	s.logProjectEvent(ctx, event.EventTypeProjectDeploy, projectID, projectModel.Name, user, metadata, "could not log project deployment action")

	err = s.updateProjectStatusAndCounts(ctx, projectID, ProjectStatusRunning)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update project status and counts after deploy", "projectId", projectID, "error", err)
	}
	return err
}
