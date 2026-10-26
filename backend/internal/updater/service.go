// Source: getarcaneapp/arcane/backend/internal/updater/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package updater

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"go.getarcane.app/kit/pkg"
	"io"
	"log/slog"

	"strings"
	"sync"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"

	"github.com/florianfinn/docklet-next/types/scheduler"
	arcaneupdater "github.com/florianfinn/docklet-next/types/updater"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/moby/moby/client"
	"github.com/samber/mo"
	"go.getarcane.app/docker"

	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"

	"go.getarcane.app/updater/refs"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	projectpkg "github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/updater/children/execution"
	"github.com/florianfinn/docklet-next/backend/internal/updater/children/recovery"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
)

// UpdaterService is Arcane's handler-facing service for the standalone updater engine.
type UpdaterService struct {
	policies    *imagePolicyService
	admission   *runs.Admission
	config      *config.Config
	coordinator *runs.Coordinator
	roles       *role.RoleService
	deps        updaterDependenciesInternal
	engine      *updater.Service
	execution   *execution.Service
	recovery    *recovery.Service
	// updateMu serializes per-container updates. docker compose's recreate
	// pipeline is not concurrency-safe for sibling containers sharing a
	// namespace. ponytail: global lock ceiling — all updates serialize; fine
	// for a UI, upgrade to per-project if batch throughput ever matters.
	updateMu sync.Mutex
}

type updaterDependenciesInternal struct {
	DB                     *database.DB
	Docker                 *dockerInternal.DockerClientService
	Settings               *settings.SettingsService
	Projects               *projectpkg.ProjectService
	ImagePuller            *image.ImageService
	ImageUpdates           *imageupdate.ImageUpdateService
	RegistryDigestResolver *registry.ContainerRegistryService
	Events                 *event.EventService
	Notifications          *notification.NotificationService
	SelfUpgrade            selfUpgradeServiceInternal
	Activity               *activity.ActivityService
	SystemUser             usertypes.Actor
	Logger                 *slog.Logger
}

type selfUpgradeServiceInternal interface {
	// TriggerUpgradeViaCLI returns the spawned upgrader container's ID, which this
	// service does not need — only update-all's manager step uses it.
	TriggerUpgradeViaCLI(ctx context.Context, user usertypes.Actor, target updater.SelfUpdateTarget) (string, error)
}

// NewUpdaterService constructs the Arcane updater facade.
func NewUpdaterService(
	db *database.DB,
	localSettings *settings.SettingsService,
	localDocker *dockerInternal.DockerClientService,
	projectService *projectpkg.ProjectService,
	imageUpdates *imageupdate.ImageUpdateService,
	registries *registry.ContainerRegistryService,
	events *event.EventService,
	imageSvc *image.ImageService,
	localNotifications *notification.NotificationService,
	upgrade selfUpgradeServiceInternal,
	activityService *activity.ActivityService,
	cfg *config.Config,
	coordinator *runs.Coordinator,
	admission *runs.Admission,
	roles *role.RoleService,
) (*UpdaterService, error) {
	service := &UpdaterService{
		config: cfg, coordinator: coordinator, admission: admission, roles: roles,
		deps: updaterDependenciesInternal{
			DB:                     db,
			Docker:                 localDocker,
			Settings:               localSettings,
			Projects:               projectService,
			ImagePuller:            imageSvc,
			ImageUpdates:           imageUpdates,
			RegistryDigestResolver: registries,
			Events:                 events,
			Notifications:          localNotifications,
			SelfUpgrade:            upgrade,
			Activity:               activityService,
			SystemUser:             usertypes.SystemUser,
		},
	}
	service.recovery = recovery.NewService(
		service.DockerClient,
		service.registryDigestResolverInternal,
		service.PendingImageUpdates,
		activityIDFromContextInternal,
		service.acquireUpdateInternal,
		service.ApplyPending,
	)
	service.execution = execution.NewService(execution.Dependencies{
		Coordinator:      coordinator,
		Config:           cfg,
		Roles:            roles,
		Activity:         activityService,
		Logger:           service.loggerInternal,
		AcquireUpdate:    service.acquireUpdateInternal,
		UpdateBusy:       errUpdateBusyInternal,
		TrackActivity:    service.trackActivityInternal,
		RunUpdate:        service.runSingleContainerUpdateInternal,
		FinishUpdate:     service.finishSingleContainerUpdateInternal,
		FreezeContainer:  service.recovery.FreezeContainer,
		ConfirmTarget:    service.recovery.ConfirmTarget,
		WithFrozenTarget: service.recovery.WithSingleTarget,
	})
	engine, err := updater.New(service.configInternal())
	if err != nil {
		return nil, fmt.Errorf("configure updater engine: %w", err)
	}
	service.engine = engine
	if projectService != nil {
		projectService.SetLifecycleChecker(service.CheckProjectUpdates)
	}
	events.SetDockerUpdatingContainers(func() []string { return engine.Status().ContainerIDs })
	return service, nil
}

func (s *UpdaterService) configInternal() updater.Config {
	return updater.Config{
		DockerClientProvider:   s,
		ImagePuller:            s,
		PendingStore:           s,
		RunRecorder:            s,
		Settings:               s,
		RegistryDigestResolver: s.registryDigestResolverInternal(),
		RegistryTagLister:      s,
		ProjectUpdater:         s,
		SelfUpdater:            s,
		Notifier:               s,
		EventRecorder:          s,
		UsedImageCollector:     updater.UsedImageCollectorFunc(s.CollectUsedImages),
		LabelPolicy:            updater.DefaultLabelPolicy(),
		SelfContainerID:        selfContainerIDInternal(),
		Logger:                 s.loggerInternal(),
	}
}

// selfContainerIDInternal returns the ID of the container Arcane runs in, so
// the updater engine routes it through the CLI self-updater even when the
// container is missing the Arcane labels. Empty when not running in Docker.
func selfContainerIDInternal() string {
	id, err := cgroup.CurrentContainerID()
	return kit.Ternary(err != nil, "", id)
}

func (s *UpdaterService) engineInternal() *updater.Service {
	return s.engine
}

func (s *UpdaterService) loggerInternal() *slog.Logger {
	if s.deps.Logger != nil {
		return s.deps.Logger
	}
	return slog.Default()
}

func (s *UpdaterService) registryDigestResolverInternal() updater.RegistryDigestResolver {
	if s == nil || s.deps.RegistryDigestResolver == nil {
		return nil
	}
	return s.deps.RegistryDigestResolver
}

// ApplyPending refuses the retired engine apply path before checks or effects.
func (s *UpdaterService) ApplyPending(ctx context.Context, options arcaneupdater.Options) (out *arcaneupdater.Result, err error) {
	return nil, imageDTO.Failure("update.path-retired", 409)
}

// applyScopedUpdatesInternal runs a scoped update into the caller's result:
// resolves the requested resources to container IDs and updates each through
// the engine's single-container path.
func (s *UpdaterService) applyScopedUpdatesInternal(ctx context.Context, options arcaneupdater.Options, out *arcaneupdater.Result) error {
	containerIDs, err := s.resolveScopedContainerIDsInternal(ctx, options)
	if err != nil {
		return err
	}
	if len(containerIDs) == 0 {
		return common.ErrUpdaterNoContainersMatched
	}

	engineOpts := moduleOptionsFromUpdaterOptionsInternal(options)
	var engineErrs []error
	for _, containerID := range containerIDs {
		target := scheduler.TargetOutcome{ID: containerID, ResourceType: "container", Status: scheduler.Running}
		if progressErr := jobcontext.Progress(ctx, target); progressErr != nil {
			return progressErr
		}
		moduleResult, engineErr := s.engineInternal().UpdateContainer(ctx, containerID, engineOpts)
		target.Status = scheduler.NeedsAttention
		if moduleResult != nil {
			partial := resultFromModuleInternal(moduleResult)
			out.Checked += partial.Checked
			out.Updated += partial.Updated
			out.Restarted += partial.Restarted
			out.Skipped += partial.Skipped
			out.Failed += partial.Failed
			out.Items = append(out.Items, partial.Items...)
			target.Status = scheduler.Succeeded
			if partial.Failed > 0 {
				target.Status = scheduler.Failed
			} else if partial.Skipped > 0 {
				target.Status = scheduler.Skipped
			}
		}
		if engineErr != nil {
			out.Failed++
			out.Items = append(out.Items, arcaneupdater.ResourceResult{
				ResourceID:   containerID,
				ResourceType: "container",
				Status:       arcaneupdater.StatusFailed,
				Error:        engineErr.Error(),
			})
			engineErrs = append(engineErrs, fmt.Errorf("%s: %w", containerID, engineErr))
			target.Status = scheduler.Failed
			target.Message = engineErr.Error()
		}
		if progressErr2 := jobcontext.Progress(ctx, target); progressErr2 != nil {
			return errors.Join(progressErr2, errors.Join(engineErrs...))
		}
	}
	s.logResultItemsInternal(ctx, out)
	out.Success = out.Failed == 0
	// Engine errors propagate like the unscoped path's engine error does —
	// the remaining containers were still attempted and recorded above.
	return errors.Join(engineErrs...)
}

// resolveScopedContainerIDsInternal maps a scoped options payload to the
// container IDs it covers.
func (s *UpdaterService) resolveScopedContainerIDsInternal(ctx context.Context, options arcaneupdater.Options) ([]string, error) {
	requested := kit.TrimNonEmpty(options.ResourceIds)
	if len(requested) == 0 {
		return nil, nil
	}

	switch strings.ToLower(strings.TrimSpace(options.Type)) {
	case "", "container":
		return requested, nil
	case "project":
		return s.containerIDsForProjectsInternal(ctx, requested)
	case "image":
		return s.containerIDsForImagesInternal(ctx, requested)
	default:
		return nil, fmt.Errorf("unsupported scoped update type %q", options.Type)
	}
}

// containerIDsForProjectsInternal resolves project IDs or compose names to
// the IDs of the containers that belong to those projects.
func (s *UpdaterService) containerIDsForProjectsInternal(ctx context.Context, projectRefs []string) ([]string, error) {
	if s.deps.Docker == nil {
		return nil, errors.New("docker service unavailable")
	}

	names := make(map[string]struct{}, len(projectRefs))
	for _, ref := range projectRefs {
		name := strings.TrimSpace(ref)
		if discoveredName, discovered := strings.CutPrefix(name, "compose:"); discovered {
			name = discoveredName
		} else if s.deps.Projects != nil {
			if project, lookupErr := s.deps.Projects.GetProjectFromDatabaseByID(ctx, ref); lookupErr == nil && project != nil {
				switch {
				case project.ComposeProjectName != nil && strings.TrimSpace(*project.ComposeProjectName) != "":
					name = *project.ComposeProjectName
				case strings.TrimSpace(project.Name) != "":
					name = project.Name
				}
			}
		}
		names[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	containers, _, _, _, err := s.deps.Docker.GetAllContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	var ids []string
	for _, summary := range containers {
		if docker.ComposeServiceLabel(summary.Labels) == "" {
			continue
		}
		project := strings.ToLower(strings.TrimSpace(docker.ComposeProjectLabel(summary.Labels)))
		if project == "" {
			continue
		}
		if _, ok := names[project]; ok {
			ids = append(ids, summary.ID)
		}
	}
	return ids, nil
}

// containerIDsForImagesInternal resolves image IDs or references to the IDs
// of the containers currently running those images.
func (s *UpdaterService) containerIDsForImagesInternal(ctx context.Context, imageRefs []string) ([]string, error) {
	if s.deps.Docker == nil {
		return nil, errors.New("docker service unavailable")
	}

	wanted := make(map[string]struct{}, len(imageRefs)*2)
	for _, ref := range imageRefs {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" {
			continue
		}
		wanted[trimmed] = struct{}{}
		if normalized := refs.NormalizeImageUpdateRef(trimmed); normalized != "" {
			wanted[normalized] = struct{}{}
		}
	}

	containers, _, _, _, err := s.deps.Docker.GetAllContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	var ids []string
	for _, summary := range containers {
		if _, ok := wanted[summary.ImageID]; ok {
			ids = append(ids, summary.ID)
			continue
		}
		if normalized := refs.NormalizeImageUpdateRef(summary.Image); normalized != "" {
			if _, ok := wanted[normalized]; ok {
				ids = append(ids, summary.ID)
			}
		}
	}
	return ids, nil
}

// UpdateSingleContainer refuses the retired standalone update path.
func (s *UpdaterService) UpdateSingleContainer(ctx context.Context, containerID string) (out *arcaneupdater.Result, err error) {
	return nil, imageDTO.Failure("update.path-retired", 409)
}

// AcceptSingleContainerUpdate refuses before persisting or dispatching work.
func (s *UpdaterService) AcceptSingleContainerUpdate(ctx context.Context, containerID string) (*activitytypes.Activity, error) {
	return nil, imageDTO.Failure("update.path-retired", 409)
}

func (s *UpdaterService) runSingleContainerUpdateInternal(ctx context.Context, containerID, activityID string) (out *arcaneupdater.Result, err error) {
	return nil, imageDTO.Failure("update.path-retired", 409)
}

// GetStatus returns the current in-memory update activity snapshot.
func (s *UpdaterService) GetStatus() arcaneupdater.Status {
	return statusFromModuleInternal(s.engineInternal().Status())
}

// GetHistory returns the most recent auto-update history records, newest first.
func (s *UpdaterService) GetHistory(ctx context.Context, limit int) ([]AutoUpdateRecord, error) {
	var records []AutoUpdateRecord
	query := s.deps.DB.WithContext(ctx).Order("start_time DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	return records, nil
}

// RestartContainersUsingOldIDs restarts containers matching old image IDs or refs.
func (s *UpdaterService) RestartContainersUsingOldIDs(ctx context.Context, oldIDToNewRef, oldRefToNewRef map[string]string) ([]arcaneupdater.ResourceResult, error) {
	results, err := s.engineInternal().RestartContainersUsingOldImages(ctx, oldIDToNewRef, oldRefToNewRef)
	return resourceResultsFromModuleInternal(results), err
}

// TriggerSelfUpdateViaCLI triggers Arcane's detached CLI self-update path.
func (s *UpdaterService) TriggerSelfUpdateViaCLI(ctx context.Context, source, containerID, containerName string, labelMap map[string]string) error {
	if !labels.IsArcaneContainer(labelMap) {
		return fmt.Errorf("%s: container is not an Arcane self-update target", source)
	}
	return s.TriggerSelfUpdate(ctx, updater.SelfUpdateTarget{
		ContainerID:   containerID,
		ContainerName: containerName,
		InstanceType:  kit.Ternary(labels.IsArcaneAgentContainer(labelMap), "agent", "server"),
		Labels:        labelMap,
	})
}

// BeginContainerUpdate marks a container as updating.
func (s *UpdaterService) BeginContainerUpdate(containerID string) func() {
	return s.engineInternal().BeginContainerUpdate(containerID)
}

// BeginProjectUpdate marks a project as updating.
func (s *UpdaterService) BeginProjectUpdate(projectID string) func() {
	return s.engineInternal().BeginProjectUpdate(projectID)
}

func (s *UpdaterService) recordAutoUpdateEventInternal(ctx context.Context, severity event.EventSeverity, metadata database.JSON) {
	if s.deps.Events == nil {
		return
	}
	phase, _ := metadata["phase"].(string)
	_, err := s.deps.Events.CreateEvent(ctx, event.CreateEventRequest{
		Type:          event.EventTypeSystemAutoUpdate,
		Severity:      severity,
		Title:         autoUpdateEventTitleInternal(phase, metadata),
		ResourceType:  mo.EmptyableToOption(strings.TrimSpace("system")).ToPointer(),
		ResourceName:  mo.EmptyableToOption(strings.TrimSpace("auto_updater")).ToPointer(),
		EnvironmentID: mo.EmptyableToOption(strings.TrimSpace("0")).ToPointer(),
		Metadata:      metadata,
	})
	if err != nil {
		s.loggerInternal().DebugContext(ctx, "failed to record auto-update event", "error", err)
	}
}

func autoUpdateEventTitleInternal(phase string, metadata database.JSON) string {
	switch phase {
	case "start":
		return "Auto-update run started"
	case "image_pull", "image":
		localImage := cmp.Or(kit.ToString(metadata["imageNew"]), kit.ToString(metadata["imageOld"]))
		return kit.Ternary(localImage != "", "Auto-update: image pull "+localImage, "Auto-update: image pull")
	case "image_prune":
		imageID := kit.ToString(metadata["imageId"])
		return kit.Ternary(imageID != "", "Auto-update: image prune "+imageID, "Auto-update: image prune")
	case "container":
		name := cmp.Or(kit.ToString(metadata["resourceName"]), kit.ToString(metadata["container"]), kit.ToString(metadata["containerId"]))
		return kit.Ternary(name != "", "Auto-update: container "+name, "Auto-update: container")
	case "project":
		name := cmp.Or(kit.ToString(metadata["projectName"]), kit.ToString(metadata["projectId"]))
		return kit.Ternary(name != "", "Auto-update: project "+name, "Auto-update: project")
	case "complete":
		return "Auto-update run completed"
	default:
		return kit.Ternary(phase != "", "Auto-update: "+phase, "Auto-update")
	}
}

// DockerClient returns Arcane's configured Docker client for the updater engine.
func (s *UpdaterService) DockerClient(ctx context.Context) (*client.Client, error) {
	if s == nil || s.deps.Docker == nil {
		return nil, common.Classify(common.ErrUnavailable, errors.New("docker service unavailable"))
	}
	return s.deps.Docker.GetClient(ctx)
}

// PullImage pulls an image through Arcane's image service. The pull is
// bounded by the dockerImagePullTimeout setting — image.ImageService.PullImage does
// not bound itself, and an unbounded engine pull would hold the auto-update
// run (and its activity slot) indefinitely.
func (s *UpdaterService) PullImage(ctx context.Context, imageRef string, progress io.Writer) error {
	if s == nil || s.deps.ImagePuller == nil {
		return common.Classify(common.ErrUnavailable, errors.New("image service unavailable"))
	}
	pulledRef, err := s.recovery.PreparePull(ctx, imageRef)
	if err != nil {
		return err
	}
	activityID := activityIDFromContextInternal(ctx)
	writer := activitylib.NewWriter(ctx, s.deps.Activity, activityID, progress, "Pulling updated images")
	defer activitylib.FlushWriter(writer)

	pullTimeoutSeconds := 0
	if s.deps.Settings != nil {
		pullTimeoutSeconds = s.deps.Settings.GetSettingsConfig().DockerImagePullTimeout.AsInt()
	}
	pullCtx, cancelPull := context.WithTimeout(ctx, timeouts.GetDuration(pullTimeoutSeconds, timeouts.DefaultDockerImagePull))
	defer cancelPull()

	if pullImageErr := s.deps.ImagePuller.PullImage(pullCtx, pulledRef, writer, s.deps.SystemUser, nil); pullImageErr != nil {
		return pullImageErr
	}
	if tagErr := s.tagFrozenPullInternal(pullCtx, pulledRef, imageRef); tagErr != nil {
		return tagErr
	}
	// Reconcile by tag so the new image's record and any project preview rows
	// for this reference stop reporting the update just installed (#4306).
	// This runs on the run context, not the pull deadline, so a pull that
	// finished late still records its result.
	if reconcileErr := s.deps.ImagePuller.ReconcilePulledImageUpdate(ctx, imageRef); reconcileErr != nil {
		slog.WarnContext(ctx, "failed to reconcile pulled image update state", "image", imageRef, "error", reconcileErr)
	}
	return nil
}

// PendingImageUpdates returns pending image update records from Arcane's database.
func (s *UpdaterService) PendingImageUpdates(ctx context.Context) ([]updater.ImageUpdateRecord, error) {
	if records, ok := s.recovery.FrozenRecords(ctx); ok {
		return records, nil
	}
	if s == nil || s.deps.DB == nil {
		return nil, common.Classify(common.ErrUnavailable, errors.New("database unavailable"))
	}

	var records []imageupdate.ImageUpdateRecord
	if err := s.deps.DB.WithContext(ctx).Where("has_update = ? AND project_id = ?", true, "").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query pending image updates: %w", err)
	}

	// Flush pending "Updates Available" notifications before the engine
	// consumes and clears these records, otherwise the notification for an
	// update applied here is silently lost (#3132). A flush that cannot
	// determine what to send aborts the run so the records survive for retry.
	if s.deps.ImageUpdates != nil {
		if err := s.deps.ImageUpdates.SendBatchUpdateNotifications(ctx); err != nil {
			return nil, err
		}
	}
	s.appendAutoUpdateActivityMessageInternal(
		ctx,
		activityIDFromContextInternal(ctx),
		fmt.Sprintf("Found %d pending image update records", len(records)),
		"Planning updates",
		10,
	)

	return s.scopedPendingRecordsInternal(ctx, records)
}

// ClearImageUpdateRecord clears a pending image update record after it is handled.
func (s *UpdaterService) ClearImageUpdateRecord(ctx context.Context, record updater.ImageUpdateRecord) error {
	if s == nil {
		return common.Classify(common.ErrUnavailable, errors.New("updater service unavailable"))
	}
	return s.clearImageUpdateRecordForModuleInternal(ctx, record)
}

// ExcludedContainers returns auto-update exclusions from Arcane settings.
func (s *UpdaterService) ExcludedContainers(ctx context.Context) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	if s.deps.Settings == nil {
		return nil, nil
	}
	return kit.Unique(kit.TrimNonEmpty(strings.Split(s.deps.Settings.GetStringSetting(ctx, "autoUpdateExcludedContainers", ""), ","))), nil
}

// ProjectByComposeName resolves an Arcane project from a Docker Compose project name.
func (s *UpdaterService) ProjectByComposeName(ctx context.Context, composeName string) (updater.ComposeProject, error) {
	if s == nil || s.deps.Projects == nil {
		return updater.ComposeProject{}, common.Classify(common.ErrUnavailable, errors.New("project service unavailable"))
	}
	project, err := s.deps.Projects.GetProjectByComposeName(ctx, composeName)
	if err != nil {
		return updater.ComposeProject{}, err
	}
	if project == nil {
		return updater.ComposeProject{}, fmt.Errorf("compose project not found: %s", composeName)
	}
	return updater.ComposeProject{ID: project.ID, Name: project.Name}, nil
}

// UpdateServices redeploys selected services through Arcane's project service.
func (s *UpdaterService) UpdateServices(ctx context.Context, projectID string, services []string) error {
	if s == nil || s.deps.Projects == nil {
		return common.Classify(common.ErrUnavailable, errors.New("project service unavailable"))
	}
	return s.deps.Projects.UpdateProjectServices(ctx, projectID, services, s.deps.SystemUser, false)
}

// TriggerSelfUpdate runs Arcane's CLI-backed self-update hook.
func (s *UpdaterService) TriggerSelfUpdate(ctx context.Context, target updater.SelfUpdateTarget) error {
	// The frozen digest is only what gets pulled; the recreated container keeps
	// the selected tag so later runs do not treat it as an immutable reference.
	if target.NewImageRef != "" {
		pinned, err := s.recovery.PreparePull(ctx, target.NewImageRef)
		if err != nil {
			return err
		}
		target.PullImageRef = pinned
	}
	if s == nil || s.deps.SelfUpgrade == nil {
		instanceType := cmp.Or(strings.TrimSpace(target.InstanceType), "server")
		return fmt.Errorf("%s self-update requires CLI upgrade service", instanceType)
	}

	// A server self-update stops this process before the run can complete its
	// activity, so annotate the activity first; startup reconciliation uses
	// the metadata flag to finalize it after the restart.
	if target.InstanceType != "agent" {
		s.markSelfUpdateTriggeredInternal(ctx, target.NewImageRef)
	}

	if _, err := s.deps.SelfUpgrade.TriggerUpgradeViaCLI(ctx, s.deps.SystemUser, target); err != nil {
		return fmt.Errorf("CLI upgrade failed: %w", err)
	}
	if progress, ok := ctx.Value(updateProgressKeyInternal{}).(*updateProgressInternal); ok {
		progress.mu.Lock()
		progress.selfTriggered = true
		progress.selfID = target.ContainerID
		progress.mu.Unlock()
	}
	return nil
}

func (s *UpdaterService) markSelfUpdateTriggeredInternal(ctx context.Context, imageRef string) {
	activityID := activityIDFromContextInternal(ctx)
	if s.deps.Activity == nil || activityID == "" {
		return
	}
	message := "Self-update initiated — Arcane will restart"
	if ref := strings.TrimSpace(imageRef); ref != "" {
		message = "Self-update initiated — Arcane will restart with " + ref
	}
	s.appendAutoUpdateActivityMessageInternal(ctx, activityID, message, "Self-update", 90)
	if err := s.deps.Activity.PatchActivityMetadata(ctx, activityID, database.JSON{"selfUpdateTriggered": true}); err != nil {
		slog.DebugContext(ctx, "failed to mark self-update on activity", "activityId", activityID, "error", err)
	}
}

// Notify buffers Arcane's container update notification when called within an
// auto-update run (see withBatchedNotificationsInternal); buffered entries are
// flushed on completion. Outside a run it sends the per-container notification.
func (s *UpdaterService) Notify(ctx context.Context, localNotification updater.Notification) error {
	if s == nil || s.deps.Notifications == nil {
		return nil
	}
	if buffer := batchedContainerUpdatesFromContextInternal(ctx); buffer != nil {
		buffer.Lock()
		buffer.entries = append(buffer.entries, notifications.ContainerUpdateBatchEntry{
			ContainerName: localNotification.ContainerName,
			ImageRef:      localNotification.ImageRef,
			OldDigest:     localNotification.OldImage,
			NewDigest:     localNotification.NewImage,
		})
		buffer.Unlock()
		return nil
	}
	return s.deps.Notifications.SendContainerUpdateNotification(
		ctx,
		localNotification.ContainerName,
		localNotification.ImageRef,
		localNotification.OldImage,
		localNotification.NewImage,
	)
}

// containerUpdateBatchInternal accumulates per-container update notifications
// so a single auto-update run produces one batched notification.
type containerUpdateBatchInternal struct {
	sync.Mutex

	entries []notifications.ContainerUpdateBatchEntry
}

type containerUpdateBatchContextKeyInternal struct{}

func batchedContainerUpdatesFromContextInternal(ctx context.Context) *containerUpdateBatchInternal {
	batch, _ := ctx.Value(containerUpdateBatchContextKeyInternal{}).(*containerUpdateBatchInternal)
	return batch
}

// flushBatchedContainerUpdatesInternal delivers the accumulated container
// update notifications as one batched notification.
func (s *UpdaterService) flushBatchedContainerUpdatesInternal(ctx context.Context, batch *containerUpdateBatchInternal) {
	if s == nil || s.deps.Notifications == nil || batch == nil {
		return
	}
	batch.Lock()
	entries := batch.entries
	batch.entries = nil
	batch.Unlock()
	if len(entries) == 0 {
		return
	}
	if err := s.deps.Notifications.SendBatchContainerUpdateNotification(ctx, entries); err != nil {
		s.loggerInternal().ErrorContext(ctx, "failed to send batched container update notification", "error", err, "count", len(entries))
	}
}
