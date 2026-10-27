// Source: getarcaneapp/arcane/backend/internal/imageupdate/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package imageupdate

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"log/slog"

	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"

	"github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/samber/mo"
	"go.getarcane.app/kit/pkg"

	"go.getarcane.app/updater/digest"
	"go.getarcane.app/updater/refs"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/tags"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/ratelimit"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/registryauth"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
)

type ImageUpdateService struct {
	db                  *database.DB
	settingsService     *settings.SettingsService
	registryService     *registry.ContainerRegistryService
	dockerService       *docker.DockerClientService
	eventService        *event.EventService
	notificationService *notification.NotificationService
	registryLimiter     *ratelimit.RegistryRateLimiter
	tags                *tags.Service
	activityService     *activity.ActivityService
	notifyMu            sync.Mutex
}

type ImageParts struct {
	Registry   string
	Repository string
	Tag        string
}

type localImageSnapshot struct {
	ImageID           string
	Repository        string
	Tag               string
	PrimaryDigest     string
	AllDigests        []string
	RepositoryDigests []string
	IsLocalBuild      bool
}

func NewImageUpdateService(
	db *database.DB,
	settingsService *settings.SettingsService,
	registryService *registry.ContainerRegistryService,
	dockerService *docker.DockerClientService,
	eventService *event.EventService,
	notificationService *notification.NotificationService,
	activityService *activity.ActivityService,
) *ImageUpdateService {
	registryLimiter := ratelimit.NewRegistryRateLimiter()
	return &ImageUpdateService{
		db:                  db,
		settingsService:     settingsService,
		registryService:     registryService,
		dockerService:       dockerService,
		eventService:        eventService,
		notificationService: notificationService,
		registryLimiter:     registryLimiter,
		tags:                tags.NewService(registryService, dockerService, settingsService, registryLimiter),
		activityService:     activityService,
	}
}

func (s *ImageUpdateService) dockerAPIContextInternal(ctx context.Context) (context.Context, context.CancelFunc) {
	timeoutSeconds := 0
	if s != nil && s.settingsService != nil {
		timeoutSeconds = s.settingsService.GetSettingsConfig().DockerAPITimeout.AsInt()
	}
	return context.WithTimeout(ctx, timeouts.GetDuration(timeoutSeconds, timeouts.DefaultDockerAPI))
}

func (s *ImageUpdateService) registryContextInternal(ctx context.Context) (context.Context, context.CancelFunc) {
	timeoutSeconds := 0
	if s != nil && s.settingsService != nil {
		timeoutSeconds = s.settingsService.GetSettingsConfig().RegistryTimeout.AsInt()
	}
	return context.WithTimeout(ctx, timeouts.GetDuration(timeoutSeconds, timeouts.DefaultRegistry))
}

func (s *ImageUpdateService) dockerClientInternal(ctx context.Context) (*client.Client, error) {
	if s == nil || s.dockerService == nil {
		return nil, errors.New("docker service unavailable")
	}
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}
	return dockerClient, nil
}

func (s *ImageUpdateService) composeBuildImageRefsInternal(ctx context.Context) (map[string]struct{}, error) {
	buildRefs := make(map[string]struct{})
	if s == nil || s.db == nil {
		return buildRefs, nil
	}

	var projectRows []struct {
		ID                 string
		BuildImageRefsJSON *string `gorm:"column:build_image_refs_json"`
	}
	if err := s.db.WithContext(ctx).
		Table("projects").
		Select("id", "build_image_refs_json").
		Where("build_image_refs_json IS NOT NULL AND build_image_refs_json <> ''").
		Find(&projectRows).Error; err != nil {
		return nil, fmt.Errorf("load project build image references: %w", err)
	}

	for i := range projectRows {
		if projectRows[i].BuildImageRefsJSON == nil {
			continue
		}
		for _, imageRef := range projects.ParseImageRefsJSON(*projectRows[i].BuildImageRefsJSON) {
			normalized := refs.NormalizeImageUpdateRef(imageRef)
			if normalized != "" {
				buildRefs[normalized] = struct{}{}
			}
		}
	}

	// Copacetic-patched tags only exist in the local daemon, so treat them like
	// locally built images instead of failing a remote registry lookup. Never
	// fail the update check over patch history.
	var patchedRefs []string
	if err := s.db.WithContext(ctx).
		Table("image_patches").
		Where("status = ?", "completed").
		Distinct().
		Pluck("patched_ref", &patchedRefs).Error; err != nil {
		slog.DebugContext(ctx, "failed to load patched image references", "error", err)
	}
	for _, imageRef := range patchedRefs {
		normalized := refs.NormalizeImageUpdateRef(imageRef)
		if normalized != "" {
			buildRefs[normalized] = struct{}{}
		}
	}

	return buildRefs, nil
}

func (s *ImageUpdateService) startImageUpdateActivityInternal(ctx context.Context, resourceName string, count int) string {
	if s.activityService == nil {
		return ""
	}
	resourceType := kit.Ternary(count > 1, "images", "image")
	localActivity, err := s.activityService.StartActivity(ctx, activity.StartActivityRequest{
		EnvironmentID: "0",
		Type:          activitytypes.TypeImageUpdateCheck,
		Queue:         true,
		ResourceType:  &resourceType,
		ResourceName:  mo.EmptyableToOption(strings.TrimSpace(resourceName)).ToPointer(),
		Step:          "Checking image updates",
		LatestMessage: "Image update check started",
		Metadata: database.JSON{
			"imageCount": count,
		},
	})
	if err != nil {
		slog.DebugContext(ctx, "failed to start image update activity", "error", err)
		return ""
	}
	return localActivity.ID
}

func (s *ImageUpdateService) appendImageUpdateActivityMessageInternal(ctx context.Context, activityID string, level activitytypes.MessageLevel, message string, progress int, step string) {
	if s.activityService == nil || activityID == "" || strings.TrimSpace(message) == "" {
		return
	}
	level = cmp.Or(level, activitytypes.MessageLevelInfo)
	if _, err := s.activityService.AppendMessage(ctx, activityID, activity.AppendActivityMessageRequest{
		Level:    level,
		Message:  message,
		Progress: &progress,
		Step:     step,
	}); err != nil {
		slog.DebugContext(ctx, "failed to append image update activity message", "activityId", activityID, "error", err)
	}
}

func (s *ImageUpdateService) completeImageUpdateActivityInternal(ctx context.Context, activityID string, success bool, message string) {
	if s.activityService == nil || activityID == "" {
		return
	}
	status := activitytypes.StatusSuccess
	var errMessage *string
	if !success {
		status = activitytypes.StatusFailed
		errMessage = mo.EmptyableToOption(strings.TrimSpace(message)).ToPointer()
		if activitylib.CancelledByContext(ctx) {
			status = activitytypes.StatusCancelled
			errMessage = nil
			message = "Image update check cancelled"
		}
	}
	message = cmp.Or(message, "Image update check completed")
	step := "Image update check complete"
	if _, err := s.activityService.CompleteActivity(utils.ActivityRuntimeContext(ctx, nil), activityID, status, message, errMessage, step); err != nil {
		// A lost terminal write strands the activity in running forever, so it
		// must be loud enough to correlate with a stuck activity panel entry.
		slog.ErrorContext(ctx, "failed to complete image update activity", "activityId", activityID, "error", err)
	}
}

func (s *ImageUpdateService) CheckImageUpdate(ctx context.Context, imageRef string) (*imageupdate.Response, error) {
	startTime := time.Now()
	activityID := s.startImageUpdateActivityInternal(ctx, imageRef, 1)
	ctx = s.activityService.Track(ctx, activityID)
	activitylib.AwaitHandlerActivitySlot(ctx, s.activityService, activityID, "0")

	if result, ok := digestPinnedImageUpdateResultInternal(imageRef).Get(); ok {
		result.ResponseTimeMs = int(time.Since(startTime).Milliseconds())
		result.ActivityID = mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()
		s.recordDigestPinnedSkipInternal(ctx, activityID, imageRef, result, 100)
		if s.eventService != nil {
			metadata := database.JSON{
				"action":         "check_update",
				"imageRef":       imageRef,
				"hasUpdate":      false,
				"updateType":     UpdateTypeDigest,
				"currentDigest":  result.CurrentDigest,
				"latestDigest":   result.LatestDigest,
				"responseTimeMs": result.ResponseTimeMs,
				"skippedReason":  "digest_pinned",
			}
			if logErr := s.eventService.LogImageEvent(ctx, event.EventTypeImageScan, "", imageRef, user.SystemUser.ID, user.SystemUser.Username, "0", metadata); logErr != nil {
				slog.WarnContext(ctx, "Failed to log digest-pinned image update check event", "imageRef", imageRef, "error", logErr.Error())
			}
		}
		s.completeImageUpdateActivityInternal(ctx, activityID, true, "Image update check skipped")
		return result, nil
	}

	s.appendImageUpdateActivityMessageInternal(ctx, activityID, activitytypes.MessageLevelInfo, "Checking "+imageRef, 20, "Checking remote digest")

	parts := s.parseImageReference(imageRef)
	if parts == nil {
		result := &imageupdate.Response{
			Error:          "Invalid image reference format",
			CheckTime:      time.Now(),
			ResponseTimeMs: int(time.Since(startTime).Milliseconds()),
			ActivityID:     mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer(),
		}
		s.completeImageUpdateActivityInternal(ctx, activityID, false, result.Error)
		return result, nil
	}

	composeBuildRefs, err := s.composeBuildImageRefsInternal(ctx)
	var digestResult *imageupdate.Response
	var snapshot *localImageSnapshot
	if err == nil {
		digestResult, snapshot, err = s.checkDigestUpdateWithSnapshotInternal(ctx, parts, composeBuildRefs)
	}
	if err == nil && digestResult == nil {
		err = errors.New("digest update check returned no result")
	}
	var tagRefs []string
	if digestResult == nil || digestResult.UpdateType != UpdateTypeLocal {
		tagRefs = append(tagRefs, imageRef)
	}
	containerUpdates, tagErr := s.checkContainerTagUpdatesInternal(ctx, tagRefs, nil)
	if tagErr != nil {
		s.completeImageUpdateActivityInternal(ctx, activityID, false, imageDTO.Neutral(tagErr, "update.check-unavailable", 503).Error())
		return digestResult, tagErr
	}
	if err != nil {
		result := &imageupdate.Response{
			Code:           checkCode(err),
			Error:          imageDTO.Neutral(err, "update.check-unavailable", 503).Error(),
			CheckTime:      time.Now(),
			ResponseTimeMs: int(time.Since(startTime).Milliseconds()),
			ActivityID:     mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer(),
		}
		metadata := database.JSON{
			"action":    "check_update",
			"imageRef":  imageRef,
			"error":     imageDTO.Neutral(err, "update.check-unavailable", 503).Error(),
			"checkType": "digest",
		}
		if logErr := s.eventService.LogImageEvent(ctx, event.EventTypeImageScan, "", imageRef, user.SystemUser.ID, user.SystemUser.Username, "0", metadata); logErr != nil {
			slog.WarnContext(ctx, "Failed to log image update check error event", "imageRef", imageRef, "error", logErr.Error())
		}
		if saveErr := s.saveUpdateResultWithSnapshotInternal(ctx, imageRef, result, snapshot); saveErr != nil {
			slog.WarnContext(ctx, "Failed to save update result", "imageRef", imageRef, "error", saveErr.Error())
		}
		tags.AttachContainerUpdates(map[string]*imageupdate.Response{imageRef: result}, containerUpdates)
		s.completeImageUpdateActivityInternal(ctx, activityID, false, result.Error)
		if result.HasUpdate {
			// A failed flush is already logged; the records stay pending for the next one.
			_ = s.SendBatchUpdateNotifications(ctx)
			return result, nil
		}
		return result, err
	}

	digestResult.ResponseTimeMs = int(time.Since(startTime).Milliseconds())
	digestResult.ActivityID = mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()
	if digestResult.UpdateType == UpdateTypeLocal {
		s.appendImageUpdateActivityMessageInternal(ctx, activityID, activitytypes.MessageLevelInfo, imageRef+" — local build, registry check skipped", 100, "Skipping image update check")
	}
	metadata := database.JSON{
		"action":         "check_update",
		"imageRef":       imageRef,
		"hasUpdate":      digestResult.HasUpdate,
		"updateType":     digestResult.UpdateType,
		"currentDigest":  digestResult.CurrentDigest,
		"latestDigest":   digestResult.LatestDigest,
		"responseTimeMs": digestResult.ResponseTimeMs,
	}
	if logErr := s.eventService.LogImageEvent(ctx, event.EventTypeImageScan, "", imageRef, user.SystemUser.ID, user.SystemUser.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "Failed to log image update check event", "imageRef", imageRef, "error", logErr.Error())
	}
	if saveErr := s.saveUpdateResultWithSnapshotInternal(ctx, imageRef, digestResult, snapshot); saveErr != nil {
		slog.WarnContext(ctx, "Failed to save update result", "imageRef", imageRef, "error", saveErr.Error())
	}

	s.notifyImageUpdateInternal(ctx, imageRef, digestResult, snapshot)
	tags.AttachContainerUpdates(map[string]*imageupdate.Response{imageRef: digestResult}, containerUpdates)
	if len(containerUpdates) > 0 {
		// A failed flush is already logged; the records stay pending for the next one.
		_ = s.SendBatchUpdateNotifications(ctx)
	}

	finalMessage := kit.Ternary(digestResult.HasUpdate, "Image update available", "Image update check completed")
	s.completeImageUpdateActivityInternal(ctx, activityID, true, finalMessage)
	return digestResult, nil
}

// notifyImageUpdateInternal sends the single-image update notification and marks the
// record notified once it was delivered to at least one provider. A partial provider
// failure still counts as notified — the user saw the update, and re-sending on every
// poll would duplicate it forever; the failure stays visible in the delivery history.
// Only a total failure (delivered == 0) leaves the record unnotified for retry.
// Prefer the snapshot's image ID: it is the same key
// saveUpdateResultWithSnapshotInternal stored the record under.
func (s *ImageUpdateService) notifyImageUpdateInternal(ctx context.Context, imageRef string, digestResult *imageupdate.Response, snapshot *localImageSnapshot) {
	if !digestResult.HasUpdate || s.notificationService == nil {
		return
	}

	delivered, notifErr := s.notificationService.SendImageUpdateNotification(ctx, imageRef, digestResult, notifications.NotificationEventImageUpdate)
	if notifErr != nil {
		slog.WarnContext(ctx, "Failed to send update notification", "imageRef", imageRef, "error", notifErr.Error())
	}
	if delivered == 0 {
		return
	}

	imageID := s.resolveNotifiedImageIDInternal(ctx, imageRef, snapshot)
	if imageID == "" {
		return
	}
	if markErr := s.MarkUpdatesAsNotified(ctx, []string{imageID}); markErr != nil {
		slog.WarnContext(ctx, "Failed to mark update as notified", "imageRef", imageRef, "error", markErr.Error())
	}
}

// resolveNotifiedImageIDInternal returns the image ID used to mark an update
// notified, preferring the snapshot's image ID and falling back to a lookup by
// reference.
func (s *ImageUpdateService) resolveNotifiedImageIDInternal(ctx context.Context, imageRef string, snapshot *localImageSnapshot) string {
	if snapshot != nil && snapshot.ImageID != "" {
		return snapshot.ImageID
	}
	resolved, err := s.getImageIDByRef(ctx, imageRef)
	if err != nil {
		slog.WarnContext(ctx, "Failed to resolve image ID to mark notified", "imageRef", imageRef, "error", imageDTO.Neutral(err, "update.check-unavailable", 503).Error())
		return ""
	}
	return resolved
}

func digestPinnedImageUpdateResultInternal(imageRef string) mo.Option[*imageupdate.Response] {
	imageRef = strings.TrimSpace(imageRef)
	pinnedDigest, ok := digest.FromReferenceSuffix(imageRef)
	if !ok {
		return mo.None[*imageupdate.Response]()
	}

	tag := "latest"
	if named, err := reference.ParseNormalizedNamed(imageRef); err == nil {
		if tagged, localOk := named.(reference.NamedTagged); localOk {
			tag = tagged.Tag()
		}
	}

	return mo.Some(&imageupdate.Response{
		HasUpdate:      false,
		UpdateType:     UpdateTypeDigest,
		CurrentVersion: tag,
		LatestVersion:  tag,
		CurrentDigest:  pinnedDigest,
		LatestDigest:   pinnedDigest,
		CheckTime:      time.Now(),
		ResponseTimeMs: 0,
	})
}

func isLocalBuildImageRefInternal(imageRef string, composeBuildRefs map[string]struct{}) bool {
	if named, err := reference.ParseNormalizedNamed(strings.TrimSpace(imageRef)); err == nil &&
		registryauth.NormalizeRegistryForComparison(reference.Domain(named)) == imageref.LocalBuildRegistry {
		return true
	}
	_, ok := composeBuildRefs[refs.NormalizeImageUpdateRef(imageRef)]
	return ok
}

func (s *ImageUpdateService) checkDigestUpdateWithSnapshotInternal(ctx context.Context, parts *ImageParts, composeBuildRefs map[string]struct{}) (*imageupdate.Response, *localImageSnapshot, error) {
	if s.registryService == nil {
		return nil, nil, errors.New("registry service unavailable")
	}

	imageRef := fmt.Sprintf("%s/%s:%s", parts.Registry, parts.Repository, parts.Tag)
	start := time.Now()
	snapshot, err := s.inspectLocalImageSnapshotInternal(ctx, imageRef, composeBuildRefs)
	if err == nil && snapshot.IsLocalBuild {
		return localBuildImageUpdateResultInternal(snapshot, int(time.Since(start).Milliseconds())), snapshot, nil
	}
	if err != nil && errdefs.IsNotFound(err) && isLocalBuildImageRefInternal(imageRef, composeBuildRefs) {
		return missingLocalBuildImageUpdateResultInternal(parts.Tag, int(time.Since(start).Milliseconds())), nil, nil
	}

	registryCtx, registryCancel := s.registryContextInternal(ctx)
	digestResult, err := s.registryService.InspectImageDigest(registryCtx, imageRef, nil)
	registryCancel()
	elapsed := time.Since(start)
	if err != nil {
		partial := digestResult // may contain auth metadata even on error
		if partial == nil {
			return nil, nil, fmt.Errorf("failed to get remote digest: %w", err)
		}
		return &imageupdate.Response{
			Code:           checkCode(err),
			Error:          imageDTO.Neutral(err, "update.check-unavailable", 503).Error(),
			CheckTime:      time.Now(),
			ResponseTimeMs: int(elapsed.Milliseconds()),
			AuthMethod:     partial.AuthMethod,
			AuthUsername:   partial.AuthUsername,
			AuthRegistry:   partial.AuthRegistry,
			UsedCredential: partial.UsedCredential,
		}, nil, fmt.Errorf("failed to get remote digest: %w", err)
	}

	if snapshot == nil {
		snapshot, err = s.inspectLocalImageSnapshotInternal(ctx, imageRef, composeBuildRefs)
		if err != nil {
			if errdefs.IsNotFound(err) {
				// The ref resolved remotely but was never pulled locally: there is
				// nothing local to compare, so report a distinct not-pulled state
				// rather than a failed check or an available update.
				return &imageupdate.Response{
					UpdateType:     UpdateTypeNotPulled,
					LatestDigest:   digestResult.Digest,
					CheckTime:      time.Now(),
					ResponseTimeMs: int(elapsed.Milliseconds()),
					AuthMethod:     digestResult.AuthMethod,
					AuthUsername:   digestResult.AuthUsername,
					AuthRegistry:   digestResult.AuthRegistry,
					UsedCredential: digestResult.UsedCredential,
				}, nil, nil
			}
			return nil, nil, fmt.Errorf("failed to get local digest: %w", err)
		}
	}

	// This comparison is deliberately index-level: local RepoDigests record the
	// multi-platform index digest a tag resolved to at pull time, and the
	// registry digest above is the same index-level value — "update available"
	// answers "would a pull fetch something new for this tag". Compose (v5.5.0+)
	// instead recreates containers based on the platform-specific manifest
	// digest in its com.docker.compose.image label, so a multi-arch tag whose
	// *other* platform changed can show an update here while a redeploy
	// correctly recreates nothing; the pull still clears the badge. Do not
	// "align" this to platform-level digests — that would require per-platform
	// registry manifest fetches for no user-visible gain.
	localDigest := snapshot.PrimaryDigest
	hasUpdate := true
	for _, localDig := range snapshot.AllDigests {
		if localDig == digestResult.Digest {
			localDigest = localDig
			hasUpdate = false
			break
		}
	}

	slog.DebugContext(ctx, "digest comparison",
		"imageRef", imageRef,
		"primaryLocalDigest", localDigest,
		"allLocalDigests", snapshot.AllDigests,
		"remoteDigest", digestResult.Digest,
		"hasUpdate", hasUpdate)

	return &imageupdate.Response{
		HasUpdate:      hasUpdate,
		UpdateType:     UpdateTypeDigest,
		CurrentDigest:  localDigest,
		LatestDigest:   digestResult.Digest,
		CheckTime:      time.Now(),
		ResponseTimeMs: int(elapsed.Milliseconds()),
		AuthMethod:     digestResult.AuthMethod,
		AuthUsername:   digestResult.AuthUsername,
		AuthRegistry:   digestResult.AuthRegistry,
		UsedCredential: digestResult.UsedCredential,
	}, snapshot, nil
}

func localBuildImageUpdateResultInternal(snapshot *localImageSnapshot, responseTimeMs int) *imageupdate.Response {
	return &imageupdate.Response{
		HasUpdate:      false,
		UpdateType:     UpdateTypeLocal,
		CurrentVersion: snapshot.Tag,
		CurrentDigest:  snapshot.PrimaryDigest,
		CheckTime:      time.Now(),
		ResponseTimeMs: responseTimeMs,
	}
}

// missingLocalBuildImageUpdateResultInternal reports a compose build ref whose
// image is not present locally: there is no registry to check against, so the
// registry lookup is skipped entirely.
func missingLocalBuildImageUpdateResultInternal(tag string, responseTimeMs int) *imageupdate.Response {
	return &imageupdate.Response{
		HasUpdate:      false,
		UpdateType:     UpdateTypeLocal,
		CurrentVersion: tag,
		CheckTime:      time.Now(),
		ResponseTimeMs: responseTimeMs,
	}
}

func (s *ImageUpdateService) parseImageReference(imageRef string) *ImageParts {
	// Use the official Docker reference parser to handle all edge cases
	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		// Fallback to manual parsing if the official parser fails
		return s.parseImageReferenceFallback(imageRef)
	}

	// Extract registry
	registryHost := reference.Domain(named)

	// Extract repository (path without registry)
	repository := reference.Path(named)

	// Extract tag or default to latest
	tag := "latest"
	if tagged, ok := named.(reference.NamedTagged); ok {
		tag = tagged.Tag()
	}

	return &ImageParts{
		Registry:   registryauth.NormalizeRegistryForComparison(registryHost),
		Repository: repository,
		Tag:        tag,
	}
}

// Fallback parser for cases where the official parser fails
func (s *ImageUpdateService) parseImageReferenceFallback(imageRef string) *ImageParts {
	var registryHost, repository, tag string
	if _, ok := digest.FromReferenceSuffix(imageRef); ok {
		digestParts := strings.Split(imageRef, "@")
		if len(digestParts) != 2 {
			return nil
		}
		repoWithRegistry := digestParts[0]
		parts := strings.Split(repoWithRegistry, "/")
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
			registryHost = parts[0]
			repository = strings.Join(parts[1:], "/")
		} else {
			registryHost = "docker.io"
			if len(parts) == 1 {
				repository = "library/" + parts[0]
			} else {
				repository = repoWithRegistry
			}
		}
		tag = "latest"
	} else {
		parts := strings.Split(imageRef, "/")
		switch {
		case len(parts) == 1:
			registryHost = "docker.io"
			if strings.Contains(parts[0], ":") {
				repoParts := strings.Split(parts[0], ":")
				repository = "library/" + repoParts[0]
				tag = repoParts[1]
			} else {
				repository = "library/" + parts[0]
				tag = "latest"
			}
		case strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":"):
			registryHost = parts[0]
			repository = strings.Join(parts[1:], "/")
			if strings.Contains(repository, ":") {
				repoParts := strings.Split(repository, ":")
				repository = repoParts[0]
				tag = repoParts[1]
			} else {
				tag = "latest"
			}
		default:
			registryHost = "docker.io"
			repository = imageRef
			if strings.Contains(repository, ":") {
				repoParts := strings.Split(repository, ":")
				repository = repoParts[0]
				tag = repoParts[1]
			} else {
				tag = "latest"
			}
		}
	}
	return &ImageParts{Registry: registryauth.NormalizeRegistryForComparison(registryHost), Repository: repository, Tag: tag}
}

func (s *ImageUpdateService) getImageRefByIDInternal(ctx context.Context, imageID string) (string, error) {
	dockerClient, err := s.dockerClientInternal(ctx)
	if err != nil {
		return "", err
	}

	imageID = strings.TrimPrefix(imageID, "sha256:")

	if imageRef, refErr := s.resolveImageRefFromInspect(ctx, dockerClient, imageID); refErr == nil {
		return imageRef, nil
	}

	// Fallback: if the image was pruned, look up the image reference from
	// running containers that were started from this image ID.
	if imageRef, refErr := s.resolveImageRefFromContainers(ctx, dockerClient, imageID); refErr == nil {
		return imageRef, nil
	}

	return "", fmt.Errorf("image not found: no local image or running container found for %s", imageID)
}

func (s *ImageUpdateService) resolveImageRefFromInspect(ctx context.Context, dockerClient client.APIClient, imageID string) (string, error) {
	apiCtx, cancel := s.dockerAPIContextInternal(ctx)
	defer cancel()

	inspectResponse, err := dockerClient.ImageInspect(apiCtx, imageID)
	if err != nil {
		return "", err
	}
	for _, tag := range inspectResponse.RepoTags {
		if tag != "<none>:<none>" {
			return tag, nil
		}
	}
	for _, digestValue := range inspectResponse.RepoDigests {
		if digestValue != "<none>@<none>" {
			if repo, _, ok := strings.Cut(digestValue, "@"); ok {
				return repo + ":latest", nil
			}
		}
	}
	return "", errors.New("no valid tags or digests")
}

func (s *ImageUpdateService) resolveImageRefFromContainers(ctx context.Context, dockerClient client.APIClient, imageID string) (string, error) {
	fullID := "sha256:" + imageID
	apiCtx, cancel := s.dockerAPIContextInternal(ctx)
	defer cancel()

	containers, err := dockerClient.ContainerList(apiCtx, client.ContainerListOptions{All: true})
	if err != nil {
		return "", err
	}
	for _, c := range containers.Items {
		if c.ImageID != fullID && c.ImageID != imageID {
			continue
		}
		if c.Image != "" && !strings.HasPrefix(c.Image, "sha256:") && !strings.Contains(c.Image, "@sha256:") {
			return c.Image, nil
		}
	}
	return "", fmt.Errorf("no container found using image %s", imageID)
}

func (s *ImageUpdateService) getAllImageRefsInternal(ctx context.Context, limit int) ([]string, error) {
	dockerClient, err := s.dockerClientInternal(ctx)
	if err != nil {
		return nil, err
	}

	imageCtx, cancelImage := s.dockerAPIContextInternal(ctx)
	imageList, err := dockerClient.ImageList(imageCtx, client.ImageListOptions{})
	cancelImage()
	if err != nil {
		return nil, fmt.Errorf("failed to list Docker images: %w", err)
	}

	containerCtx, cancelContainers := s.dockerAPIContextInternal(ctx)
	containerList, err := dockerClient.ContainerList(containerCtx, client.ContainerListOptions{All: true})
	cancelContainers()
	if err != nil {
		slog.WarnContext(ctx, "failed to list Docker containers; continuing without update-check opt-out filtering", "error", imageDTO.Neutral(err, "update.check-unavailable", 503).Error())
		return imageRefsFromSummariesInternal(imageList.Items, limit), nil
	}

	return filterImageSummariesByContainerOptOutInternal(imageList.Items, containerList.Items, limit), nil
}

func imageRefsFromSummariesInternal(images []image.Summary, limit int) []string {
	seen := make(map[string]struct{})
	imageRefs := make([]string, 0)

	for _, summary := range images {
		for _, imageRef := range summary.RepoTags {
			if imageRef == "<none>:<none>" {
				continue
			}
			if _, exists := seen[imageRef]; exists {
				continue
			}

			seen[imageRef] = struct{}{}
			imageRefs = append(imageRefs, imageRef)
			if limit > 0 && len(imageRefs) >= limit {
				return imageRefs
			}
		}
	}

	return imageRefs
}

// monitoringEligibilityInternal records, per image ID and normalized
// reference, whether at least one consuming container still permits update
// checks. Only the update-check label is consulted: containers excluded from
// automatic installation keep being monitored.
type monitoringEligibilityInternal struct {
	// True when at least one container using the image ID or reference has
	// not opted out; present but false when every such container opted out.
	byImageID   map[string]bool
	byRef       map[string]bool
	byContainer map[string]bool
}

func newMonitoringEligibilityInternal(containers []container.Summary) monitoringEligibilityInternal {
	eligibility := monitoringEligibilityInternal{byImageID: map[string]bool{}, byRef: map[string]bool{}, byContainer: map[string]bool{}}
	for _, summary := range containers {
		eligible := !imageref.IsUpdateCheckDisabled(summary.Labels)
		eligibility.byContainer[summary.ID] = eligible
		if imageID := strings.TrimSpace(summary.ImageID); imageID != "" {
			eligibility.byImageID[imageID] = eligibility.byImageID[imageID] || eligible
		}
		if normalizedRef := refs.NormalizeImageUpdateRef(summary.Image); normalizedRef != "" {
			eligibility.byRef[normalizedRef] = eligibility.byRef[normalizedRef] || eligible
		}
	}
	return eligibility
}

// imageEligible reports whether an image may be checked: unused images are
// eligible, used images need one consumer that has not opted out.
func (e monitoringEligibilityInternal) imageEligible(imageID, normalizedRef string) bool {
	eligibleByID, usedByID := e.byImageID[strings.TrimSpace(imageID)]
	eligibleRef, usedByRef := e.byRef[normalizedRef]
	return (!usedByID && !usedByRef) || eligibleByID || eligibleRef
}

// recordEligible reports whether a stored result may still be reported. Results
// scoped to a container that no longer exists are stale and not eligible.
func (e monitoringEligibilityInternal) recordEligible(record *ImageUpdateRecord) bool {
	if record.ContainerID != "" {
		return e.byContainer[record.ContainerID]
	}
	return e.imageEligible(record.ID, refs.NormalizeImageUpdateRef(record.Repository+":"+record.Tag))
}

func filterImageSummariesByContainerOptOutInternal(images []image.Summary, containers []container.Summary, limit int) []string {
	eligibility := newMonitoringEligibilityInternal(containers)

	seen := make(map[string]struct{})
	filtered := make([]string, 0)

	// Equivalent spellings share one candidate; the first one seen is kept.
	// Digest-pinned refs do not normalize, so they dedupe on their own spelling.
	addCandidate := func(imageRef, imageID string) bool {
		imageRef = strings.TrimSpace(imageRef)
		if imageRef == "" || imageRef == "<none>:<none>" || refs.IsImageIDLikeReference(imageRef) {
			return false
		}
		key := cmp.Or(refs.NormalizeImageUpdateRef(imageRef), imageRef)
		if _, exists := seen[key]; exists {
			return false
		}

		if !eligibility.imageEligible(imageID, key) {
			return false
		}

		seen[key] = struct{}{}
		filtered = append(filtered, imageRef)
		return limit > 0 && len(filtered) >= limit
	}

	for _, summary := range images {
		for _, imageRef := range summary.RepoTags {
			if addCandidate(imageRef, summary.ID) {
				return filtered
			}
		}
	}

	// Containers can run images whose tag is no longer present locally, so
	// their references are candidates too.
	for _, summary := range containers {
		if addCandidate(summary.Image, summary.ImageID) {
			return filtered
		}
	}

	return filtered
}
