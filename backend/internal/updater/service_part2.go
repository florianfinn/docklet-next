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

	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/containerregistry"

	"github.com/florianfinn/docklet-next/types/scheduler"
	arcaneupdater "github.com/florianfinn/docklet-next/types/updater"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/samber/mo"
	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"
	"go.getarcane.app/kit/pkg"

	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"

	"go.getarcane.app/updater/refs"

	"github.com/florianfinn/docklet-next/backend/internal/common"

	"github.com/florianfinn/docklet-next/backend/internal/database"

	"github.com/florianfinn/docklet-next/backend/internal/event"

	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"

	projectpkg "github.com/florianfinn/docklet-next/backend/internal/project"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"

	"github.com/florianfinn/docklet-next/backend/pkg/utils"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
)

// RecordEvent records updater lifecycle events in Arcane's event stream.
func (s *UpdaterService) RecordEvent(ctx context.Context, evt updater.Event) error {
	if s == nil {
		return nil
	}

	eventType, ok := containerEventTypeInternal(evt.Phase).Get()
	if ok {
		if s.deps.Events == nil {
			return nil
		}
		return s.deps.Events.LogContainerEvent(
			ctx,
			eventType,
			evt.ResourceID,
			evt.ResourceName,
			s.deps.SystemUser.ID,
			s.deps.SystemUser.Username,
			"0",
			evt.Metadata,
		)
	}

	severity := kit.Ternary(strings.EqualFold(evt.Severity, "error"), event.EventSeverityError, event.EventSeverityInfo)
	s.recordAutoUpdateEventInternal(ctx, severity, database.JSON{
		"phase":        evt.Phase,
		"resourceId":   evt.ResourceID,
		"resourceName": evt.ResourceName,
		"resourceType": evt.ResourceType,
		"time":         time.Now().UTC().Format(time.RFC3339),
	})
	return nil
}

func containerEventTypeInternal(phase string) mo.Option[event.EventType] {
	switch phase {
	case "container_stop":
		return mo.Some(event.EventTypeContainerStop)
	case "container_delete":
		return mo.Some(event.EventTypeContainerDelete)
	case "container_create":
		return mo.Some(event.EventTypeContainerCreate)
	case "container_start":
		return mo.Some(event.EventTypeContainerStart)
	case "container_update":
		return mo.Some(event.EventTypeContainerUpdate)
	default:
		return mo.None[event.EventType]()
	}
}

type activityIDContextKeyInternal struct{}

func contextWithActivityIDInternal(ctx context.Context, activityID string) context.Context {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return ctx
	}
	return context.WithValue(ctx, activityIDContextKeyInternal{}, activityID)
}

func activityIDFromContextInternal(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	activityID, _ := ctx.Value(activityIDContextKeyInternal{}).(string)
	return strings.TrimSpace(activityID)
}

func (s *UpdaterService) startAutoUpdateActivityInternal(ctx context.Context, dryRun bool) string {
	if s.deps.Activity == nil {
		return ""
	}
	localActivity, err := s.deps.Activity.StartActivity(ctx, activitylib.StartRequest{
		EnvironmentID: "0",
		Type:          activitytypes.TypeAutoUpdate,
		Queue:         true,
		ResourceType:  mo.EmptyableToOption(strings.TrimSpace("system")).ToPointer(),
		ResourceName:  mo.EmptyableToOption(strings.TrimSpace("Auto update")).ToPointer(),
		Step:          "Planning updates",
		LatestMessage: "Auto-update run started",
		Metadata:      database.JSON{"dryRun": dryRun},
	})
	if err != nil {
		slog.DebugContext(ctx, "failed to start auto-update activity", "error", err)
		return ""
	}
	return localActivity.ID
}

func (s *UpdaterService) startSingleContainerUpdateActivityInternal(ctx context.Context, containerID string, tracked bool) (*activitytypes.Activity, context.Context, error) {
	if s.deps.Activity == nil {
		return nil, ctx, nil
	}
	name := containerID
	lookupCtx, cancelLookup := context.WithTimeout(ctx, 2*time.Second)
	if dockerClient, dockerErr := s.DockerClient(lookupCtx); dockerErr == nil && dockerClient != nil {
		if inspected, inspectErr := compat.ContainerInspectWithCompatibility(lookupCtx, dockerClient, containerID, client.ContainerInspectOptions{}); inspectErr == nil {
			if actualName := strings.TrimPrefix(strings.TrimSpace(inspected.Container.Name), "/"); actualName != "" {
				name = actualName
			}
		}
	}
	cancelLookup()
	user, _ := userctx.CurrentUserFromContext(ctx)
	if initiator, ok := utils.UpdateInitiatorFromContext(ctx); ok {
		user = &usertypes.Actor{Username: initiator.Username}
		user.ID = initiator.UserID
		if initiator.DisplayName != "" {
			user.DisplayName = &initiator.DisplayName
		}
	}
	request := activitylib.StartRequest{
		EnvironmentID: "0",
		Type:          activitytypes.TypeAutoUpdate,
		Queue:         true,
		DeferSlot:     true,
		ResourceType:  mo.EmptyableToOption(strings.TrimSpace("container")).ToPointer(),
		ResourceID:    &containerID,
		ResourceName:  &name,
		StartedBy:     user,
		Step:          "Updating container",
		LatestMessage: "Container update started",
		Metadata:      database.JSON{"containerID": containerID},
	}
	var item *activitytypes.Activity
	workCtx := ctx
	var err error
	if tracked {
		item, workCtx, err = s.deps.Activity.StartTrackedActivity(ctx, request)
	} else {
		item, err = s.deps.Activity.StartActivity(ctx, request)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("start container update activity: %w", err)
	}
	return item, workCtx, nil
}

func (s *UpdaterService) finishSingleContainerUpdateInternal(ctx context.Context, activityID string, result *arcaneupdater.Result, runErr error) {
	if s.deps.Activity == nil || activityID == "" {
		return
	}
	metadata, message := singleContainerActivitySummaryInternal(result)
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if containerName, ok := metadata["containerName"].(string); ok && containerName != "" {
		if _, err := s.deps.Activity.UpdateActivity(writeCtx, activityID, activitylib.UpdateRequest{ResourceName: &containerName}); err != nil {
			slog.WarnContext(writeCtx, "failed to update container activity name", "activityId", activityID, "error", err)
		}
	}
	if err := s.deps.Activity.PatchActivityMetadata(writeCtx, activityID, metadata); err != nil {
		slog.WarnContext(writeCtx, "failed to persist container update result", "activityId", activityID, "error", err)
	}
	if runErr == nil && result != nil && result.Failed == 0 {
		if result.Updated > 0 || result.Restarted > 0 {
			message = "Container updated"
		}
		if _, err := s.deps.Activity.CompleteActivity(writeCtx, activityID, activitytypes.StatusSuccess, message, nil); err != nil {
			slog.ErrorContext(writeCtx, "failed to complete container update activity", "activityId", activityID, "error", err)
		}
		return
	}
	if runErr == nil && result == nil {
		runErr = errors.New("container update produced no result")
	}
	if runErr == nil && result != nil && len(result.Items) > 0 && result.Items[0].Error != "" {
		runErr = errors.New(result.Items[0].Error)
	}
	s.completeAutoUpdateActivityInternal(ctx, activityID, result, runErr)
}

func singleContainerActivitySummaryInternal(result *arcaneupdater.Result) (database.JSON, string) {
	metadata := database.JSON{}
	message := "Container update completed"
	if result == nil {
		return metadata, message
	}
	metadata["updated"] = result.Updated
	metadata["restarted"] = result.Restarted
	metadata["skipped"] = result.Skipped
	metadata["failed"] = result.Failed
	if len(result.Items) > 0 {
		item := result.Items[0]
		metadata["updateOutcome"] = item.Status
		if item.ResourceName != "" {
			metadata["containerName"] = item.ResourceName
		}
		if item.Error != "" {
			metadata["updateReason"] = item.Error
			if item.Status == string(updater.StatusSkipped) {
				return metadata, "Container update skipped: " + item.Error
			}
		}
	}
	if result.Skipped > 0 {
		return metadata, "Container update skipped"
	}
	if result.Updated == 0 && result.Restarted == 0 && result.Failed == 0 {
		message = "Container already current"
	}
	return metadata, message
}

func (s *UpdaterService) appendAutoUpdateActivityMessageInternal(ctx context.Context, activityID, message, step string, progress int) {
	if s.deps.Activity == nil || strings.TrimSpace(activityID) == "" {
		return
	}
	if strings.TrimSpace(step) == "" {
		step = message
	}
	if _, err := s.deps.Activity.AppendMessage(ctx, activityID, activitylib.AppendMessageRequest{
		Level:    activitytypes.MessageLevelInfo,
		Message:  message,
		Progress: &progress,
		Step:     step,
	}); err != nil {
		slog.DebugContext(ctx, "failed to append auto-update activity message", "activityId", activityID, "error", err)
	}
}

func (s *UpdaterService) completeAutoUpdateActivityInternal(ctx context.Context, activityID string, result *arcaneupdater.Result, applyErr error) {
	if s.deps.Activity == nil || strings.TrimSpace(activityID) == "" {
		return
	}

	status := activitytypes.StatusSuccess
	message := "Auto-update run completed"
	var errMessage *string
	if applyErr != nil {
		status = activitytypes.StatusFailed
		errText := applyErr.Error()
		errMessage = &errText
		message = errText
	} else if result != nil && result.Failed > 0 {
		status = activitytypes.StatusFailed
		errText := fmt.Sprintf("%d update action(s) failed", result.Failed)
		errMessage = &errText
		message = errText
	}
	if status == activitytypes.StatusFailed && activitylib.CancelledByContext(ctx) {
		status = activitytypes.StatusCancelled
		message = "Auto-update cancelled"
		errMessage = nil
	}

	if _, err := s.deps.Activity.CompleteActivity(utils.ActivityRuntimeContext(ctx, nil), activityID, status, message, errMessage); err != nil {
		// A lost terminal write strands the activity in running forever, so it
		// must be loud enough to correlate with a stuck activity panel entry.
		slog.ErrorContext(ctx, "failed to complete auto-update activity", "activityId", activityID, "error", err)
	}
}

func (s *UpdaterService) trackActivityInternal(ctx context.Context, activityID string) context.Context {
	if s.deps.Activity == nil || strings.TrimSpace(activityID) == "" {
		return ctx
	}
	return s.deps.Activity.Track(ctx, activityID)
}

func imageUpdateRecordToModuleInternal(record imageupdate.ImageUpdateRecord) updater.ImageUpdateRecord {
	return updater.ImageUpdateRecord{
		ID:             record.ID,
		ContainerID:    record.ContainerID,
		Repository:     record.Repository,
		Tag:            record.Tag,
		HasUpdate:      record.HasUpdate,
		UpdateType:     updater.UpdateType(record.UpdateType),
		CurrentVersion: record.CurrentVersion,
		LatestVersion:  record.LatestVersion,
		CurrentDigest:  record.CurrentDigest,
		LatestDigest:   record.LatestDigest,
		CheckTime:      record.CheckTime,
		LastError:      record.LastError,
	}
}

// moduleOptionsFromUpdaterOptionsInternal narrows Arcane's request options to
// what the engine acts on. Options.Type and Options.ResourceIds stay behind:
// the engine never read them, and ApplyPending already routes a scoped request
// through applyScopedUpdatesInternal before reaching the engine. Settings
// exclusions always apply to these runs; only UpdateSingleContainer overrides
// them for its explicitly requested container.
func moduleOptionsFromUpdaterOptionsInternal(options arcaneupdater.Options) updater.Options {
	return updater.Options{
		Force:  options.ForceUpdate,
		DryRun: options.DryRun,
	}
}

// resultFromModuleInternal converts an engine result to Arcane's wire type. The
// engine reports times as time.Time and a Duration method; Arcane's API has
// always carried them as strings, so they are formatted here. ActivityID is not
// an engine concept; every caller sets it on the returned value.
func resultFromModuleInternal(result *updater.Result) *arcaneupdater.Result {
	if result == nil {
		return &arcaneupdater.Result{Items: []arcaneupdater.ResourceResult{}}
	}
	return &arcaneupdater.Result{
		Success:   result.Success,
		Checked:   result.Checked,
		Updated:   result.Updated,
		Restarted: result.Restarted,
		Skipped:   result.Skipped,
		Failed:    result.Failed,
		StartTime: formatModuleTimeInternal(result.StartTime),
		EndTime:   formatModuleTimeInternal(result.EndTime),
		Duration:  result.Duration().String(),
		Items:     resourceResultsFromModuleInternal(result.Items),
	}
}

func formatModuleTimeInternal(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func resourceResultsFromModuleInternal(results []updater.ResourceResult) []arcaneupdater.ResourceResult {
	out := make([]arcaneupdater.ResourceResult, 0, len(results))
	for _, result := range results {
		out = append(out, resourceResultFromModuleInternal(result))
	}
	return out
}

// resourceResultFromModuleInternal converts one engine result to Arcane's wire
// type. The engine now reports a single old/new image; Arcane's API carries
// maps, which only ever held the "main" entry, so that shape is rebuilt here.
func resourceResultFromModuleInternal(result updater.ResourceResult) arcaneupdater.ResourceResult {
	mainImage := func(ref string) map[string]string {
		if ref == "" {
			return nil
		}
		return map[string]string{"main": ref}
	}
	return arcaneupdater.ResourceResult{
		ResourceID:      result.ResourceID,
		ResourceName:    result.ResourceName,
		ResourceType:    string(result.ResourceType),
		Status:          string(result.Status),
		UpdateAvailable: result.UpdateAvailable,
		UpdateApplied:   result.UpdateApplied,
		OldImages:       mainImage(result.OldImage),
		NewImages:       mainImage(result.NewImage),
		Error:           result.Error,
		Details:         result.Details,
	}
}

func statusFromModuleInternal(status updater.Status) arcaneupdater.Status {
	return arcaneupdater.Status{
		UpdatingContainers: status.UpdatingContainers,
		UpdatingProjects:   status.UpdatingProjects,
		ContainerIds:       status.ContainerIDs,
		ProjectIds:         status.ProjectIDs,
	}
}

func (s *UpdaterService) recordRunInternal(ctx context.Context, item arcaneupdater.ResourceResult) error {
	now := time.Now()
	record := &AutoUpdateRecord{
		ResourceID:       item.ResourceID,
		ResourceType:     item.ResourceType,
		ResourceName:     item.ResourceName,
		Status:           AutoUpdateStatus(item.Status),
		StartTime:        now,
		EndTime:          &now,
		UpdateAvailable:  item.UpdateAvailable || item.Status == string(updater.StatusUpdated) || item.Status == string(updater.StatusUpdateAvailable),
		UpdateApplied:    item.UpdateApplied,
		OldImageVersions: mapToJSONInternal(item.OldImages),
		NewImageVersions: mapToJSONInternal(item.NewImages),
		Details:          detailsToJSONInternal(item.Details),
	}
	if item.Error != "" {
		record.Error = &item.Error
	}
	return s.deps.DB.WithContext(ctx).Create(record).Error
}

func (s *UpdaterService) clearImageUpdateRecordForModuleInternal(ctx context.Context, record updater.ImageUpdateRecord) error {
	if s.deps.DB == nil {
		return nil
	}

	if record.ContainerID != "" && !strings.HasPrefix(record.ID, "container::") {
		return s.clearUnscopedRecordInternal(ctx, record)
	}
	query := s.deps.DB.WithContext(ctx).Model(&imageupdate.ImageUpdateRecord{})
	if record.ContainerID != "" {
		query = query.Where("container_id = ?", record.ContainerID)
	} else {
		query = query.Where("container_id = ?", "")
	}
	if strings.TrimSpace(record.ID) != "" {
		return query.Where("id = ?", record.ID).Update("has_update", false).Error
	}
	return query.Where("repository = ? AND tag = ?", record.Repository, record.Tag).Update("has_update", false).Error
}

func mapToJSONInternal(values map[string]string) database.JSON {
	if len(values) == 0 {
		return nil
	}
	out := make(database.JSON, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func detailsToJSONInternal(values map[string]any) database.JSON {
	if len(values) == 0 {
		return nil
	}
	out := make(database.JSON, len(values))
	maps.Copy(out, values)
	return out
}

func (s *UpdaterService) logResultItemsInternal(ctx context.Context, result *arcaneupdater.Result) {
	if result == nil {
		return
	}
	for _, item := range result.Items {
		severity := event.EventSeverityInfo
		switch item.Status {
		case string(updater.StatusFailed):
			severity = event.EventSeverityError
		case string(updater.StatusUpdated):
			severity = event.EventSeveritySuccess
		}
		s.recordAutoUpdateEventInternal(ctx, severity, database.JSON{
			"phase":        item.ResourceType,
			"resourceId":   item.ResourceID,
			"resourceName": item.ResourceName,
			"status":       item.Status,
			"error":        item.Error,
			"oldImages":    item.OldImages,
			"newImages":    item.NewImages,
		})
	}
}

// CollectUsedImages returns normalized image references used by running Arcane resources.
func (s *UpdaterService) CollectUsedImages(ctx context.Context) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	var errs []error
	successfulSources := 0

	if s.deps.Docker == nil {
		errs = append(errs, common.Classify(common.ErrUnavailable, errors.New("docker service unavailable")))
	} else {
		dcli, err := s.deps.Docker.GetClient(ctx)
		if err != nil || dcli == nil {
			if err == nil {
				err = common.Classify(common.ErrUnavailable, errors.New("docker client unavailable"))
			}
			errs = append(errs, err)
			s.loggerInternal().DebugContext(ctx, "collectUsedImages: docker connection unavailable", "error", err)
		} else if collectUsedImagesFromContainersErr := s.collectUsedImagesFromContainersInternal(ctx, dcli, out); collectUsedImagesFromContainersErr != nil {
			errs = append(errs, collectUsedImagesFromContainersErr)
			s.loggerInternal().DebugContext(ctx, "collectUsedImages: failed collecting from containers", "error", collectUsedImagesFromContainersErr)
		} else {
			successfulSources++
		}
	}

	if s.deps.Projects != nil {
		if err := s.collectUsedImagesFromProjectsInternal(ctx, out); err != nil {
			errs = append(errs, err)
			s.loggerInternal().DebugContext(ctx, "collectUsedImages: failed collecting from projects", "error", err)
		} else {
			successfulSources++
		}
	}

	if successfulSources == 0 {
		return nil, errors.Join(errs...)
	}

	s.loggerInternal().DebugContext(ctx, "collectUsedImages: collected used images", "count", len(out))
	return out, nil
}

func (s *UpdaterService) collectUsedImagesFromContainersInternal(ctx context.Context, dcli *client.Client, out map[string]struct{}) error {
	if dcli == nil {
		return nil
	}

	excludedContainers := s.buildExcludedContainerSetInternal(ctx)
	listResult, err := dcli.ContainerList(ctx, client.ContainerListOptions{All: false})
	if err != nil {
		return err
	}

	for _, summary := range listResult.Items {
		if labels.IsUpdateDisabled(summary.Labels) {
			s.loggerInternal().DebugContext(ctx, "collectUsedImagesFromContainers: container opted out by labels", "containerId", summary.ID)
			continue
		}

		if docker.ContainerNameExcluded(summary.Names, excludedContainers) {
			s.loggerInternal().DebugContext(ctx, "collectUsedImagesFromContainers: skipping excluded container", "containerId", summary.ID, "names", summary.Names)
			continue
		}

		imageRef := strings.TrimSpace(summary.Image)
		if imageRef != "" && !refs.IsImageIDLikeReference(imageRef) {
			addNormalizedImageUpdateRefInternal(ctx, out, imageRef, "collectUsedImagesFromContainers", "containerId", summary.ID)
			continue
		}

		inspectResult, inspectErr := compat.ContainerInspectWithCompatibility(ctx, dcli, summary.ID, client.ContainerInspectOptions{})
		if inspectErr != nil {
			s.loggerInternal().DebugContext(ctx, "collectUsedImagesFromContainers: container inspect failed", "containerId", summary.ID, "error", inspectErr)
			continue
		}
		inspect := inspectResult.Container
		if inspect.Config != nil && labels.IsUpdateDisabled(inspect.Config.Labels) {
			s.loggerInternal().DebugContext(ctx, "collectUsedImagesFromContainers: container inspect labels opted out", "containerId", summary.ID)
			continue
		}
		for _, tag := range s.normalizedTagsForContainerInternal(ctx, dcli, inspect) {
			out[tag] = struct{}{}
		}
	}
	return nil
}

func (s *UpdaterService) collectUsedImagesFromComposeContainersInternal(ctx context.Context, composeContainers []container.Summary, activeProjectNames, out map[string]struct{}) {
	for _, summary := range composeContainers {
		projectName := docker.ComposeProjectLabel(summary.Labels)
		if projectName == "" {
			continue
		}
		if _, isActive := activeProjectNames[projectName]; !isActive {
			continue
		}
		if labels.IsUpdateDisabled(summary.Labels) {
			continue
		}

		imageRef := strings.TrimSpace(summary.Image)
		if imageRef == "" || refs.IsImageIDLikeReference(imageRef) {
			continue
		}
		addNormalizedImageUpdateRefInternal(ctx, out, imageRef, "collectUsedImagesFromComposeContainers", "containerId", summary.ID)
	}
}

func (s *UpdaterService) normalizedTagsForContainerInternal(ctx context.Context, dcli *client.Client, inspect container.InspectResponse) []string {
	seen := map[string]struct{}{}

	if dcli != nil {
		if imageInspect, err := dcli.ImageInspect(ctx, inspect.Image); err == nil {
			for _, tag := range imageInspect.RepoTags {
				if strings.TrimSpace(tag) == "" || tag == "<none>:<none>" {
					continue
				}
				addNormalizedImageUpdateRefInternal(ctx, seen, tag, "normalizedTagsForContainer repo tag", "imageId", inspect.Image)
			}
		}
	}

	if inspect.Config != nil && inspect.Config.Image != "" {
		addNormalizedImageUpdateRefInternal(ctx, seen, inspect.Config.Image, "normalizedTagsForContainer config image", "imageId", inspect.Image)
	}

	out := slices.Collect(maps.Keys(seen))
	return out
}

func (s *UpdaterService) buildExcludedContainerSetInternal(ctx context.Context) map[string]bool {
	if s.deps.Settings == nil {
		return nil
	}
	return docker.ExcludedContainerNameSet(s.deps.Settings.GetStringSetting(ctx, "autoUpdateExcludedContainers", ""))
}

func (s *UpdaterService) collectUsedImagesFromProjectsInternal(ctx context.Context, out map[string]struct{}) error {
	if s.deps.Projects == nil {
		return nil
	}

	allProjects, err := s.deps.Projects.ListAllProjects(ctx)
	if err != nil {
		return err
	}

	activeProjectNames := activeComposeProjectNameSetInternal(allProjects)
	if len(activeProjectNames) == 0 {
		return nil
	}

	var dockerClient client.APIClient
	if s.deps.Docker != nil {
		cli, cliErr := s.deps.Docker.GetClient(ctx)
		if cliErr != nil {
			return cliErr
		}
		dockerClient = cli
	}
	composeContainers, err := projects.ListGlobalComposeContainers(ctx, dockerClient, s.deps.Docker.DockerHost())
	if err != nil {
		return err
	}

	s.collectUsedImagesFromComposeContainersInternal(ctx, composeContainers, activeProjectNames, out)
	return nil
}

func activeComposeProjectNameSetInternal(items []projectpkg.Project) map[string]struct{} {
	active := make(map[string]struct{})
	for _, project := range items {
		if project.IsArchived {
			continue
		}
		if project.Status != projectpkg.ProjectStatusRunning && project.Status != projectpkg.ProjectStatusPartiallyRunning {
			continue
		}

		name := strings.TrimSpace(project.Name)
		if name == "" {
			continue
		}
		active[name] = struct{}{}
		if normalized := projects.NormalizeProjectName(name); normalized != "" {
			active[normalized] = struct{}{}
		}
	}
	return active
}

func addNormalizedImageUpdateRefInternal(ctx context.Context, out map[string]struct{}, imageRef, source string, attrs ...any) {
	normalizedRef := refs.NormalizeImageUpdateRef(imageRef)
	if normalizedRef != "" {
		out[normalizedRef] = struct{}{}
		return
	}

	args := slices.Clone(attrs)
	args = append(args, "source", source, "imageRef", imageRef)
	slog.DebugContext(ctx, "skipping invalid image reference", args...)
}

func (s *UpdaterService) tagFrozenPullInternal(ctx context.Context, pulledRef, imageRef string) error {
	if pulledRef == imageRef {
		return nil
	}
	dockerClient, err := s.DockerClient(ctx)
	if err != nil {
		return err
	}
	_, err = dockerClient.ImageTag(ctx, client.ImageTagOptions{Source: pulledRef, Target: imageRef})
	return err
}

type updateAdmissionKeyInternal struct{}

var errUpdateBusyInternal = errors.New("another container update is running")

func (s *UpdaterService) acquireUpdateInternal(ctx context.Context) (context.Context, func(), error) {
	if s.admission == nil || ctx.Value(updateAdmissionKeyInternal{}) == s {
		return ctx, func() {}, nil
	}
	lease, admitted, err := s.admission.TryAcquire(ctx, scheduler.AdmissionKey{Scope: "updater"})
	if err != nil {
		return ctx, nil, err
	}
	if !admitted {
		return ctx, nil, errUpdateBusyInternal
	}
	return context.WithValue(ctx, updateAdmissionKeyInternal{}, s), func() { lease.Release(ctx) }, nil
}

type (
	updateProgressKeyInternal struct{}
	updateProgressInternal    struct {
		mu            sync.Mutex
		err           error
		selfTriggered bool
		selfID        string
	}
)

// RecordUpdateRun persists one updater resource result into Arcane history.
func (s *UpdaterService) RecordUpdateRun(ctx context.Context, result updater.ResourceResult) error {
	var recordErr error
	if s != nil && s.deps.DB != nil {
		recordErr = s.recordRunInternal(ctx, resourceResultFromModuleInternal(result))
	}
	name := cmp.Or(strings.TrimSpace(result.ResourceName), result.ResourceID)
	message := name + ": " + string(result.Status)
	level := activitytypes.MessageLevelInfo
	status := scheduler.NeedsAttention
	switch result.Status {
	case updater.StatusUpdated, updater.StatusRestarted, updater.StatusUpToDate:
		status = scheduler.Succeeded
		if result.Status != updater.StatusUpToDate && result.OldImage != "" && result.NewImage != "" && result.OldImage != result.NewImage {
			message += " (" + result.OldImage + " -> " + result.NewImage + ")"
		}
	case updater.StatusSkipped:
		status = scheduler.Skipped
	case updater.StatusChecked, updater.StatusUpdateAvailable:
		status = scheduler.NeedsAttention
	case updater.StatusFailed:
		status = scheduler.Failed
		level = activitytypes.MessageLevelError
	}
	if reason := strings.TrimSpace(result.Error); reason != "" {
		if level == activitytypes.MessageLevelError {
			message = name + ": " + reason
		} else {
			message += " (" + reason + ")"
		}
	}
	if activityID := activityIDFromContextInternal(ctx); activityID != "" && s != nil && s.deps.Activity != nil {
		if _, appendErr := s.deps.Activity.AppendMessage(ctx, activityID, activitylib.AppendMessageRequest{Level: level, Message: message, Step: "Applying updates"}); appendErr != nil {
			slog.DebugContext(ctx, "failed to append update result activity message", "activityId", activityID, "resource", name, "error", appendErr)
		}
	}
	var evidenceErr error
	if status == scheduler.Succeeded {
		evidenceErr = s.recovery.VerifyResult(ctx, result.ResourceID)
	}
	if evidenceErr != nil {
		status = scheduler.NeedsAttention
	}
	progressMessage := result.Error
	if evidenceErr != nil {
		progressMessage = evidenceErr.Error()
	}
	progressErr := jobcontext.Progress(
		ctx,
		scheduler.TargetOutcome{
			ID: result.ResourceID,
			ResourceType: string(
				result.ResourceType,
			),
			Status:  status,
			Message: progressMessage,
			ActivityID: activityIDFromContextInternal(
				ctx,
			),
		},
	)
	err := errors.Join(recordErr, progressErr, evidenceErr)
	if progress, ok := ctx.Value(updateProgressKeyInternal{}).(*updateProgressInternal); ok && err != nil {
		progress.mu.Lock()
		progress.err = errors.Join(progress.err, err)
		progress.mu.Unlock()
	}
	return err
}

func (p *updateProgressInternal) completeBatchInternal(ctx context.Context, options arcaneupdater.Options, out *arcaneupdater.Result, batchCompleted bool, err error) error {
	activityID := activityIDFromContextInternal(ctx)
	p.mu.Lock()
	err = errors.Join(err, p.err)
	recordingFailed := p.err != nil
	_, durableRun := jobcontext.Run(ctx)
	selfTriggered := p.selfTriggered && durableRun
	selfID := p.selfID
	p.mu.Unlock()
	status := scheduler.Succeeded
	if !batchCompleted && err == nil {
		err = errors.New("update batch ended without a confirmed result")
		recordingFailed = true
	}
	if err != nil || out.Failed > 0 {
		status = scheduler.Partial
	}
	if recordingFailed {
		status = scheduler.NeedsAttention
	}
	if selfTriggered {
		err = errors.Join(
			err,
			jobcontext.Progress(
				ctx,
				scheduler.TargetOutcome{
					ID:           selfID,
					ResourceType: "container",
					Status:       scheduler.NeedsAttention,
					ActivityID:   activityID,
					Message:      "Self-update completion requires review",
				},
			),
		)
		status = scheduler.NeedsAttention
		err = errors.Join(err, errors.New("self-update was triggered but completion requires review"))
	}
	batchType := updateBatchTypeInternal(ctx, options, batchCompleted)
	checkpoint := scheduler.TargetOutcome{ID: "auto-update", ResourceType: batchType, Status: status, ActivityID: activityID}
	if err != nil {
		checkpoint.Message = err.Error()
	}
	checkpointErr := jobcontext.Progress(ctx, checkpoint)
	err = errors.Join(err, checkpointErr)
	if err != nil {
		out.Success = false
	}
	if recordingFailed || selfTriggered || checkpointErr != nil {
		err = &scheduler.OutcomeError{Outcome: scheduler.Outcome{Status: scheduler.NeedsAttention, Message: err.Error(), ActivityID: activityID}, Cause: err}
	}
	return err
}

// ListTags supplies Arcane's registry credentials to the updater's tag checker.
func (s *UpdaterService) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	if s.deps.RegistryDigestResolver == nil {
		return nil, errors.New("registry service unavailable")
	}
	var credentials []containerregistry.Credential
	if s.deps.Projects != nil {
		resolved, err := s.deps.Projects.ResolveRegistryCredentials(ctx)
		if err != nil {
			return nil, err
		}
		credentials = resolved
	}
	return s.deps.RegistryDigestResolver.ListImageTags(ctx, imageRef, credentials)
}
