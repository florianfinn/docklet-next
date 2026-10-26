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
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	"github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/florianfinn/docklet-next/types/user"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/samber/mo"

	"go.getarcane.app/sys/crypto"
	"go.getarcane.app/updater/digest"
	"go.getarcane.app/updater/refs"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"

	"github.com/florianfinn/docklet-next/backend/internal/event"

	"github.com/florianfinn/docklet-next/backend/internal/registry"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
)

func (s *ImageUpdateService) inspectLocalImageSnapshotInternal(ctx context.Context, imageRef string, composeBuildRefs map[string]struct{}) (*localImageSnapshot, error) {
	dockerClient, err := s.dockerClientInternal(ctx)
	if err != nil {
		return nil, err
	}

	apiCtx, cancel := s.dockerAPIContextInternal(ctx)
	defer cancel()

	inspectResponse, err := dockerClient.ImageInspect(apiCtx, imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}

	var allDigests, repositoryDigests []string
	var primaryDigest string
	isLocalBuild := false
	requested, referenceErr := refs.NormalizeReference(imageRef)

	// Extract all digests from RepoDigests
	if len(inspectResponse.RepoDigests) > 0 {
		for _, repoDigest := range inspectResponse.RepoDigests {
			digestValue, ok := digest.FromReferenceSuffix(repoDigest)
			if !ok {
				continue
			}

			allDigests = append(allDigests, digestValue)
			repositoryRef, parseErr := refs.NormalizeReference(repoDigest)
			if referenceErr == nil && parseErr == nil && repositoryRef.RegistryHost == requested.RegistryHost && repositoryRef.Repository == requested.Repository {
				repositoryDigests = append(repositoryDigests, digestValue)
			}

			// Use first digest as primary if not yet set
			primaryDigest = cmp.Or(primaryDigest, digestValue)
		}
	}

	if len(repositoryDigests) > 0 {
		primaryDigest = repositoryDigests[0]
	}

	// Fallback to image ID if no repo digests available
	if primaryDigest == "" {
		isLocalBuild = true
		primaryDigest = inspectResponse.ID
		allDigests = []string{primaryDigest}
	}
	isLocalBuild = isLocalBuild || isLocalBuildImageRefInternal(imageRef, composeBuildRefs)

	repo, tag := extractRepoAndTagFromImage(inspectResponse.InspectResponse)
	tag = tagWithFallbackInternal(tag, s.parseImageReference(imageRef))

	return &localImageSnapshot{
		ImageID:           inspectResponse.ID,
		Repository:        repo,
		Tag:               tag,
		PrimaryDigest:     primaryDigest,
		AllDigests:        allDigests,
		RepositoryDigests: repositoryDigests,
		IsLocalBuild:      isLocalBuild,
	}, nil
}

func (s *ImageUpdateService) CheckImageUpdateByID(ctx context.Context, imageID string) (*imageupdate.Response, error) {
	imageRef, err := s.getImageRefByIDInternal(ctx, imageID)
	if err != nil {
		metadata := database.JSON{
			"action":  "check_update_by_id",
			"imageID": imageID,
			"error":   imageDTO.Neutral(err, "update.check-unavailable", 503).Error(),
		}
		if logErr := s.eventService.LogImageEvent(ctx, event.EventTypeImageScan, imageID, "", user.SystemUser.ID, user.SystemUser.Username, "0", metadata); logErr != nil {
			slog.WarnContext(ctx, "Failed to log image update check by ID error event", "imageId", imageID, "error", logErr.Error())
		}
		return nil, fmt.Errorf("failed to get image reference: %w", err)
	}
	result, err := s.CheckImageUpdate(ctx, imageRef)
	if err != nil {
		return nil, err
	}
	if len(result.ContainerUpdates) == 0 {
		if saveErr := s.saveUpdateResultByIDInternal(ctx, imageID, result, s.parseImageReference(imageRef)); saveErr != nil {
			slog.WarnContext(ctx, "Failed to save update result by ID", "imageId", imageID, "error", saveErr.Error())
		}
	}
	return result, nil
}

func (s *ImageUpdateService) saveUpdateResultWithSnapshotInternal(ctx context.Context, imageRef string, result *imageupdate.Response, snapshot *localImageSnapshot) error {
	if snapshot != nil && snapshot.ImageID != "" {
		return s.savePreparedUpdateResultInternal(ctx, snapshot.ImageID, snapshot.Repository, snapshot.Tag, result)
	}

	parts := s.parseImageReference(imageRef)
	if parts == nil {
		return errors.New("invalid image reference")
	}
	imageID, err := s.getImageIDByRef(ctx, imageRef)
	if err != nil {
		repository := buildImageUpdateRepositoryInternal(parts)
		syntheticID := fmt.Sprintf("ref::%s@%s", strings.ToLower(strings.TrimSpace(repository)), strings.TrimSpace(parts.Tag))
		slog.DebugContext(ctx, "Saving image update result with synthetic ref ID",
			"imageRef", imageRef,
			"error", imageDTO.Neutral(err, "update.check-unavailable", 503).Error(),
			"repository", repository,
			"tag", parts.Tag,
			"syntheticId", syntheticID)
		// Persist registry results even when the local image no longer exists. This keeps
		// project/image update status available for pruned images using a ref-scoped record.
		return s.savePreparedUpdateResultInternal(ctx, syntheticID, repository, parts.Tag, result)
	}

	return s.saveUpdateResultByIDInternal(ctx, imageID, result, parts)
}

func buildImageUpdateRepositoryInternal(parts *ImageParts) string {
	if parts == nil {
		return ""
	}

	repository := strings.TrimSpace(parts.Repository)
	if parts.Registry == "docker.io" && repository != "" && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}
	if strings.TrimSpace(parts.Registry) == "" {
		return repository
	}

	return fmt.Sprintf("%s/%s", strings.TrimSpace(parts.Registry), repository)
}

func countBatchResultOutcomesInternal(imageRefs []string, results map[string]*imageupdate.Response) (int, int) {
	successCount := 0
	errorCount := 0

	for _, imageRef := range imageRefs {
		result := results[imageRef]
		hasError := result == nil
		if result != nil {
			hasError = strings.TrimSpace(result.Error) != ""
			for _, containerUpdate := range result.ContainerUpdates {
				hasError = hasError || containerUpdate.Error != ""
			}
		}
		if !hasError {
			successCount++
			continue
		}
		errorCount++
	}

	return successCount, errorCount
}

// imageCheckResultMessageInternal derives an activity message level and text from
// a per-image update check result: errors become ERROR, available updates become
// SUCCESS, and up-to-date images stay INFO.
func imageCheckResultMessageInternal(imageRef string, res *imageupdate.Response) (activitytypes.MessageLevel, string) {
	if res == nil {
		return activitytypes.MessageLevelError, imageRef + ": check failed"
	}
	if err := strings.TrimSpace(res.Error); err != "" {
		return activitytypes.MessageLevelError, fmt.Sprintf("%s: %s", imageRef, err)
	}
	if res.UpdateType == UpdateTypeLocal {
		return activitytypes.MessageLevelInfo, imageRef + " — local build, registry check skipped"
	}
	if res.UpdateType == UpdateTypeNotPulled {
		return activitytypes.MessageLevelInfo, imageRef + " — not pulled locally, no local digest to compare"
	}
	if res.HasUpdate {
		return activitytypes.MessageLevelSuccess, imageRef + " — update available"
	}
	return activitytypes.MessageLevelInfo, imageRef + " — up to date"
}

func extractRepoAndTagFromImage(dockerImage image.InspectResponse) (repo, tag string) {
	if len(dockerImage.RepoTags) > 0 && dockerImage.RepoTags[0] != "<none>:<none>" {
		if named, err := reference.ParseNormalizedNamed(dockerImage.RepoTags[0]); err == nil {
			repo = reference.FamiliarName(named)
			if tagged, ok := named.(reference.NamedTagged); ok {
				tag = tagged.Tag()
			} else {
				tag = "latest"
			}
			return repo, tag
		}

		parts := strings.SplitN(dockerImage.RepoTags[0], ":", 2)
		repo = parts[0]
		if len(parts) > 1 {
			tag = parts[1]
		} else {
			tag = "latest"
		}
		return repo, tag
	}

	if len(dockerImage.RepoDigests) > 0 {
		for _, rd := range dockerImage.RepoDigests {
			if rd == "<none>@<none>" {
				continue
			}
			if repoCandidate, _, found := strings.CutLast(rd, "@"); found && repoCandidate != "" {
				return repoCandidate, "<none>"
			}
		}
	}

	return "<none>", "<none>"
}

func buildImageUpdateRecord(imageID, repo, tag string, result *imageupdate.Response) *ImageUpdateRecord {
	currentVersion := cmp.Or(result.CurrentVersion, tag)

	return &ImageUpdateRecord{
		ID:             imageID,
		Repository:     repo,
		Tag:            tag,
		HasUpdate:      result.HasUpdate,
		UpdateType:     result.UpdateType,
		CurrentVersion: currentVersion,
		LatestVersion:  mo.EmptyableToOption(result.LatestVersion).ToPointer(),
		CurrentDigest:  mo.EmptyableToOption(result.CurrentDigest).ToPointer(),
		LatestDigest:   mo.EmptyableToOption(result.LatestDigest).ToPointer(),
		CheckTime:      result.CheckTime,
		ResponseTimeMs: result.ResponseTimeMs,
		LastError:      mo.EmptyableToOption(result.Error).ToPointer(),
		AuthMethod:     mo.EmptyableToOption(result.AuthMethod).ToPointer(),
		AuthUsername:   mo.EmptyableToOption(result.AuthUsername).ToPointer(),
		AuthRegistry:   mo.EmptyableToOption(result.AuthRegistry).ToPointer(),
		UsedCredential: result.UsedCredential,
	}
}

func repositoryCandidatesSliceInternal(candidates map[string]struct{}) []string {
	if len(candidates) == 0 {
		return nil
	}

	repositories := slices.Collect(maps.Keys(candidates))
	return repositories
}

func savePreparedUpdateResultWithTxInternal(tx *gorm.DB, imageID, repo, tag string, result *imageupdate.Response) error {
	updateRecord := buildImageUpdateRecord(imageID, repo, tag, result)

	// Check if there's an existing record to compare state changes
	var existingRecord ImageUpdateRecord
	hasExisting := tx.Where("id = ?", imageID).First(&existingRecord).Error == nil

	// A registry rate limit (429) is transient: keep the previous good result
	// instead of clobbering it with an error record
	if hasExisting &&
		strings.TrimSpace(result.Error) != "" &&
		(result.Code == "registry.rate-limited" || registry.IsRateLimitErrorString(result.Error)) &&
		strings.TrimSpace(mo.PointerToOption(existingRecord.LastError).OrEmpty()) == "" {
		slog.DebugContext(tx.Statement.Context, "Preserving previous image update result; check hit a registry rate limit",
			"imageId", imageID, "repository", repo, "tag", tag, "error", result.Error)
		return nil
	}

	if hasExisting {
		// Existing record found - check if we need to reset notification_sent
		stateChanged := existingRecord.HasUpdate != updateRecord.HasUpdate
		digestChanged := mo.PointerToOption(existingRecord.LatestDigest).OrEmpty() != mo.PointerToOption(updateRecord.LatestDigest).OrEmpty()
		versionChanged := mo.PointerToOption(existingRecord.LatestVersion).OrEmpty() != mo.PointerToOption(updateRecord.LatestVersion).OrEmpty()

		// Reset notification_sent if the update state changed in any way
		if stateChanged || (updateRecord.HasUpdate && (digestChanged || versionChanged)) {
			updateRecord.NotificationSent = false
		} else {
			// Keep the existing notification_sent value if nothing changed
			updateRecord.NotificationSent = existingRecord.NotificationSent
		}
	} else {
		// New record - start with notification_sent = false
		updateRecord.NotificationSent = false
	}

	return tx.Save(updateRecord).Error
}

func (s *ImageUpdateService) saveUpdateResultByIDInternal(ctx context.Context, imageID string, result *imageupdate.Response, fallback *ImageParts) error {
	sanitizeCheckResult(result)
	dockerClient, err := s.dockerClientInternal(ctx)
	if err != nil {
		return err
	}

	apiCtx, cancel := s.dockerAPIContextInternal(ctx)
	defer cancel()

	dockerImage, err := dockerClient.ImageInspect(apiCtx, imageID)
	if err != nil {
		return fmt.Errorf("failed to inspect image: %w", err)
	}

	repo, tag := extractRepoAndTagFromImage(dockerImage.InspectResponse)
	tag = tagWithFallbackInternal(tag, fallback)
	return s.savePreparedUpdateResultInternal(ctx, imageID, repo, tag, result)
}

func tagWithFallbackInternal(tag string, fallback *ImageParts) string {
	if tag == "<none>" && fallback != nil && strings.TrimSpace(fallback.Tag) != "" {
		return fallback.Tag
	}
	return tag
}

func (s *ImageUpdateService) savePreparedUpdateResultInternal(ctx context.Context, imageID, repo, tag string, result *imageupdate.Response) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return savePreparedUpdateResultWithTxInternal(tx, imageID, repo, tag, result)
	})
}

func (s *ImageUpdateService) getImageIDByRef(ctx context.Context, imageRef string) (string, error) {
	dockerClient, err := s.dockerClientInternal(ctx)
	if err != nil {
		return "", err
	}

	apiCtx, cancel := s.dockerAPIContextInternal(ctx)
	defer cancel()

	inspectResponse, err := dockerClient.ImageInspect(apiCtx, imageRef)
	if err != nil {
		return "", fmt.Errorf("image not found: %w", err)
	}
	return inspectResponse.ID, nil
}

func (s *ImageUpdateService) markLegacyImageRefAfterPull(ctx context.Context, imageRef string) error {
	if s.db == nil {
		return nil
	}

	snapshot, err := s.inspectLocalImageSnapshotInternal(ctx, imageRef, nil)
	if err != nil {
		return fmt.Errorf("inspect pulled image: %w", err)
	}

	checkTime := time.Now().UTC()
	result := &imageupdate.Response{
		HasUpdate:      false,
		UpdateType:     UpdateTypeDigest,
		CurrentVersion: snapshot.Tag,
		LatestVersion:  snapshot.Tag,
		CurrentDigest:  snapshot.PrimaryDigest,
		LatestDigest:   snapshot.PrimaryDigest,
		CheckTime:      checkTime,
		ResponseTimeMs: 0,
	}

	_, tag, repositoryCandidates, hasLookup := imageref.ParseUpdateLookup(imageRef)
	projectChecks, confirmedDigest, err := s.projectChecksAfterPull(ctx, imageRef, tag, repositoryCandidatesSliceInternal(repositoryCandidates), snapshot.RepositoryDigests)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if hasLookup {
			repositories := repositoryCandidatesSliceInternal(repositoryCandidates)
			if len(repositories) > 0 {
				// Only clear synthetic ref:: records, not real sha256 image ID records.
				// Clearing sha256 records would incorrectly mark containers that are still
				// running the old image as up-to-date (see: #2453).
				if clearStaleUpdatesErr := tx.Model(&ImageUpdateRecord{}).
					Where("id LIKE 'ref::%' AND tag = ? AND repository IN ?", tag, repositories).
					Update("has_update", false).Error; clearStaleUpdatesErr != nil {
					return fmt.Errorf("clear stale image updates: %w", clearStaleUpdatesErr)
				}
			}
		}
		for _, record := range projectChecks {
			// Record the local digest the preview targeted when the pull carries it.
			pulledDigest := cmp.Or(confirmedDigest, snapshot.RepositoryDigests[0])
			if previewDigest := mo.PointerToOption(record.LatestDigest).OrEmpty(); confirmedDigest == "" && slices.Contains(snapshot.RepositoryDigests, previewDigest) {
				pulledDigest = previewDigest
			}
			// Preserve any preview replaced while the registry lookup was in flight.
			if refreshProjectChecksErr := tx.Model(&ImageUpdateRecord{}).
				Where(map[string]any{
					"id": record.ID, "check_time": record.CheckTime, "policy_key": record.PolicyKey,
					"latest_digest": record.LatestDigest, "latest_version": record.LatestVersion,
					"current_digest": record.CurrentDigest, "last_error": record.LastError, "has_update": record.HasUpdate,
				}).
				Updates(map[string]any{
					"has_update":      false,
					"update_type":     UpdateTypeDigest,
					"current_version": tag,
					"latest_version":  tag,
					"current_digest":  pulledDigest,
					"latest_digest":   pulledDigest,
					"check_time":      checkTime,
				}).Error; refreshProjectChecksErr != nil {
				return fmt.Errorf("refresh project update checks: %w", refreshProjectChecksErr)
			}
		}

		if savePreparedUpdateResultWithTxErr := savePreparedUpdateResultWithTxInternal(tx, snapshot.ImageID, snapshot.Repository, snapshot.Tag, result); savePreparedUpdateResultWithTxErr != nil {
			return fmt.Errorf("save pulled image update state: %w", savePreparedUpdateResultWithTxErr)
		}

		return nil
	})
}

// projectChecksAfterPull returns the digest previews a pull of imageRef
// satisfies. Previews that were disabled, local, or not pulled carry no digest
// result and are left for the next project check.
func (s *ImageUpdateService) projectChecksAfterPull(ctx context.Context, imageRef, tag string, repositories, localDigests []string) ([]ImageUpdateRecord, string, error) {
	if len(repositories) == 0 || len(localDigests) == 0 {
		return nil, "", nil
	}
	var records []ImageUpdateRecord
	if err := s.db.WithContext(ctx).
		Where("project_id <> '' AND update_type = ? AND tag = ? AND repository IN ?", UpdateTypeDigest, tag, repositories).
		Where("latest_version IS NULL OR latest_version = '' OR latest_version = tag").
		Where("last_error IS NULL OR last_error = ''").Find(&records).Error; err != nil {
		return nil, "", fmt.Errorf("load project update checks after pull: %w", err)
	}
	digestMismatch := func(record ImageUpdateRecord) bool {
		latestDigest := mo.PointerToOption(record.LatestDigest).OrEmpty()
		return latestDigest != "" && !slices.Contains(localDigests, latestDigest)
	}
	if !slices.ContainsFunc(records, digestMismatch) {
		return records, "", nil
	}

	// Digests have no ordering. Confirm a moved tag without using the digest cache.
	// A registry failure leaves the differing previews untouched rather than
	// blocking the reconciliation of the pulled image state.
	if s.registryService != nil {
		registryCtx, cancel := s.registryContextInternal(ctx)
		defer cancel()
		latest, err := s.registryService.InspectImageDigest(registryCtx, imageRef, nil)
		if err != nil {
			slog.WarnContext(ctx, "Failed to verify pulled image against project previews", "imageRef", imageRef, "error", imageDTO.Neutral(err, "update.check-unavailable", 503).Error())
		} else if latest != nil && slices.Contains(localDigests, latest.Digest) {
			return records, latest.Digest, nil
		}
	}
	return slices.DeleteFunc(records, digestMismatch), "", nil
}

func (s *ImageUpdateService) StoredUpdateByImageID(ctx context.Context, imageID string) (*ImageUpdateRecord, bool, error) {
	imageID = strings.TrimSpace(imageID)
	if s == nil || s.db == nil || imageID == "" {
		return nil, false, nil
	}

	var record ImageUpdateRecord
	if err := s.db.WithContext(ctx).Where("id = ?", imageID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get stored image update by image id: %w", err)
	}

	// This legacy version-reader port carries T only. Native keys (including
	// classic config digests) must never escape as immutable pull references.
	record.HasUpdate = false
	record.LatestDigest = record.LegacyPullDigest()
	return &record, true, nil
}

// GetUnnotifiedUpdates returns a map of image IDs that have updates but haven't been notified yet
func (s *ImageUpdateService) GetUnnotifiedUpdates(ctx context.Context) (map[string]*ImageUpdateRecord, error) {
	var records []ImageUpdateRecord
	if err := s.db.WithContext(ctx).
		Where("has_update = ? AND notification_sent = ? AND project_id = ?", true, false, "").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to get unnotified updates: %w", err)
	}

	result := make(map[string]*ImageUpdateRecord)
	for i := range records {
		result[records[i].ID] = &records[i]
	}
	return result, nil
}

// filterUnnotifiedByMonitoringInternal drops records whose resource no longer
// permits update checks, using the same eligibility rules as discovery. A
// failed container listing is returned so nothing is sent or marked.
func (s *ImageUpdateService) filterUnnotifiedByMonitoringInternal(ctx context.Context, records map[string]*ImageUpdateRecord) error {
	dockerClient, err := s.dockerClientInternal(ctx)
	if err != nil {
		return fmt.Errorf("resolve update-check eligibility: %w", err)
	}
	apiCtx, cancel := s.dockerAPIContextInternal(ctx)
	listed, err := dockerClient.ContainerList(apiCtx, client.ContainerListOptions{All: true})
	cancel()
	if err != nil {
		return fmt.Errorf("list containers for update-check eligibility: %w", err)
	}
	eligibility := newMonitoringEligibilityInternal(listed.Items)
	for id, record := range records {
		if !eligibility.recordEligible(record) {
			slog.DebugContext(ctx, "Skipping update notification for resource opted out of update checks", "recordId", id)
			delete(records, id)
		}
	}
	return nil
}

// MarkUpdatesAsNotified marks the given image IDs as having been notified
func (s *ImageUpdateService) MarkUpdatesAsNotified(ctx context.Context, imageIDs []string) error {
	if len(imageIDs) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).
		Model(&ImageUpdateRecord{}).
		Where("id IN ?", imageIDs).
		Update("notification_sent", true).Error
}

type batchImage struct {
	refs         []string
	canonicalRef string
	parts        *ImageParts
}

type batchImageProgressRecorder struct {
	mu        sync.Mutex
	results   map[string]*imageupdate.Response
	completed int
	total     int
}

func (r *batchImageProgressRecorder) recordInternal(localRefs []string, res *imageupdate.Response) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.completed++
	progress := 10 + int(float64(r.completed)/float64(r.total)*80)
	for _, imageRef := range localRefs {
		r.results[imageRef] = res
	}
	return progress
}

func (s *ImageUpdateService) parseAndGroupImagesInternal(imageRefs []string) (map[string]map[string]struct{}, map[string]*imageupdate.Response, []batchImage) {
	regRepos := make(map[string]map[string]struct{})
	results := make(map[string]*imageupdate.Response)
	var images []batchImage
	indexByNormalizedRef := make(map[string]int)

	for _, imageRef := range imageRefs {
		if result, ok := digestPinnedImageUpdateResultInternal(imageRef).Get(); ok {
			results[imageRef] = result
			continue
		}

		parts := s.parseImageReference(imageRef)
		if parts == nil {
			results[imageRef] = &imageupdate.Response{
				Error:          "Invalid image reference format",
				CheckTime:      time.Now(),
				ResponseTimeMs: 0,
			}
			continue
		}
		if _, ok := regRepos[parts.Registry]; !ok {
			regRepos[parts.Registry] = make(map[string]struct{})
		}
		regRepos[parts.Registry][parts.Repository] = struct{}{}
		normalizedRef := strings.ToLower(fmt.Sprintf("%s/%s:%s", parts.Registry, parts.Repository, parts.Tag))
		if idx, exists := indexByNormalizedRef[normalizedRef]; exists {
			images[idx].refs = append(images[idx].refs, imageRef)
			continue
		}

		indexByNormalizedRef[normalizedRef] = len(images)
		images = append(images, batchImage{
			refs:         []string{imageRef},
			canonicalRef: imageRef,
			parts:        parts,
		})
	}
	return regRepos, results, images
}

func (
	s *ImageUpdateService,
) checkSingleImageInBatchInternal(
	ctx context.Context,
	externalCreds []containerregistry.Credential,
	parts *ImageParts,
	composeBuildRefs map[string]struct{},
) (
	*imageupdate.Response,
	*localImageSnapshot,
) {
	if s.registryService == nil {
		return &imageupdate.Response{
			Error:          "registry service unavailable",
			CheckTime:      time.Now(),
			ResponseTimeMs: 0,
		}, nil
	}

	start := time.Now()
	imageRef := fmt.Sprintf("%s/%s:%s", parts.Registry, parts.Repository, parts.Tag)
	snapshot, ldErr := s.inspectLocalImageSnapshotInternal(ctx, imageRef, composeBuildRefs)
	if ldErr == nil && snapshot.IsLocalBuild {
		return localBuildImageUpdateResultInternal(snapshot, int(time.Since(start).Milliseconds())), snapshot
	}
	if ldErr != nil && errdefs.IsNotFound(ldErr) && isLocalBuildImageRefInternal(imageRef, composeBuildRefs) {
		return missingLocalBuildImageUpdateResultInternal(parts.Tag, int(time.Since(start).Milliseconds())), nil
	}

	registryCtx, registryCancel := s.registryContextInternal(ctx)
	digestResult, digestErr := s.registryService.InspectImageDigest(registryCtx, imageRef, externalCreds)
	registryCancel()
	if digestErr != nil {
		resp := &imageupdate.Response{
			Code:           checkCode(digestErr),
			Error:          imageDTO.Neutral(digestErr, "update.check-unavailable", 503).Error(),
			CheckTime:      time.Now(),
			ResponseTimeMs: int(time.Since(start).Milliseconds()),
		}
		if digestResult != nil {
			resp.AuthMethod = digestResult.AuthMethod
			resp.AuthUsername = digestResult.AuthUsername
			resp.AuthRegistry = digestResult.AuthRegistry
			resp.UsedCredential = digestResult.UsedCredential
		}
		return resp, nil
	}

	if ldErr != nil {
		snapshot, ldErr = s.inspectLocalImageSnapshotInternal(ctx, imageRef, composeBuildRefs)
		if ldErr != nil {
			if errdefs.IsNotFound(ldErr) {
				// The ref resolved remotely but was never pulled locally (e.g. a
				// stopped project whose compose pin changed): there is nothing
				// local to compare, so report a distinct not-pulled state rather
				// than a failed check or an available update.
				return &imageupdate.Response{
					UpdateType:     UpdateTypeNotPulled,
					LatestDigest:   digestResult.Digest,
					CheckTime:      time.Now(),
					ResponseTimeMs: int(time.Since(start).Milliseconds()),
					AuthMethod:     digestResult.AuthMethod,
					AuthUsername:   digestResult.AuthUsername,
					AuthRegistry:   digestResult.AuthRegistry,
					UsedCredential: digestResult.UsedCredential,
				}, nil
			}
			return &imageupdate.Response{
				Code:           checkCode(ldErr),
				Error:          imageDTO.Neutral(ldErr, "update.check-unavailable", 503).Error(),
				CheckTime:      time.Now(),
				ResponseTimeMs: int(time.Since(start).Milliseconds()),
				AuthMethod:     digestResult.AuthMethod,
				AuthUsername:   digestResult.AuthUsername,
				AuthRegistry:   digestResult.AuthRegistry,
				UsedCredential: digestResult.UsedCredential,
			}, nil
		}
	}

	localDigest := snapshot.PrimaryDigest
	hasDigestUpdate := true
	for _, localDig := range snapshot.AllDigests {
		if localDig == digestResult.Digest {
			localDigest = localDig
			hasDigestUpdate = false
			break
		}
	}

	return &imageupdate.Response{
		HasUpdate:      hasDigestUpdate,
		UpdateType:     UpdateTypeDigest,
		CurrentDigest:  localDigest,
		LatestDigest:   digestResult.Digest,
		CheckTime:      time.Now(),
		ResponseTimeMs: int(time.Since(start).Milliseconds()),
		AuthMethod:     digestResult.AuthMethod,
		AuthUsername:   digestResult.AuthUsername,
		AuthRegistry:   digestResult.AuthRegistry,
		UsedCredential: digestResult.UsedCredential,
	}, snapshot
}

func (s *ImageUpdateService) resolveBatchCredentialsInternal(ctx context.Context, externalCreds []containerregistry.Credential) []containerregistry.Credential {
	if len(externalCreds) > 0 {
		filtered := make([]containerregistry.Credential, 0, len(externalCreds))
		for _, cred := range externalCreds {
			if !cred.Enabled || strings.TrimSpace(cred.URL) == "" || strings.TrimSpace(cred.Username) == "" || strings.TrimSpace(cred.Token) == "" {
				continue
			}
			filtered = append(filtered, cred)
		}
		return filtered
	}

	if s.registryService == nil {
		return nil
	}

	registries, err := s.registryService.GetEnabledRegistries(ctx)
	if err != nil {
		slog.DebugContext(ctx, "failed to load enabled registries for batch check", "error", imageDTO.Neutral(err, "update.check-unavailable", 503).Error())
		return nil
	}

	credentials := make([]containerregistry.Credential, 0, len(registries))
	for _, reg := range registries {
		if strings.TrimSpace(reg.URL) == "" || strings.TrimSpace(reg.Username) == "" || reg.Token == "" {
			continue
		}

		token, decryptErr := crypto.Decrypt(reg.Token)
		if decryptErr != nil {
			slog.DebugContext(ctx, "failed to decrypt registry token for batch check", "registryUrl", reg.URL, "error", decryptErr.Error())
			continue
		}
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		credentials = append(credentials, containerregistry.Credential{
			URL:      reg.URL,
			Username: reg.Username,
			Token:    token,
			Enabled:  reg.Enabled,
		})
	}

	return credentials
}

func (
	s *ImageUpdateService,
) checkBatchImageInternal(
	ctx context.Context,
	activityID string,
	resolvedCreds []containerregistry.Credential,
	img batchImage,
	composeBuildRefs map[string]struct{},
	recorder *batchImageProgressRecorder,
) error {
	registryRecord := img.parts.Registry

	if err := s.registryLimiter.Acquire(ctx, registryRecord); err != nil {
		slog.DebugContext(ctx, "skipping image check: registry limiter acquire failed",
			"imageRef", img.canonicalRef,
			"registry", registryRecord,
			"error", imageDTO.Neutral(err, "update.check-unavailable", 503).Error())
		if ctx.Err() != nil {
			return ctx.Err()
		}

		res := &imageupdate.Response{
			Code:           checkCode(err),
			Error:          imageDTO.Neutral(err, "update.check-unavailable", 503).Error(),
			CheckTime:      time.Now(),
			ResponseTimeMs: 0,
			ActivityID:     mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer(),
		}
		s.recordBatchImageCheckInternal(ctx, activityID, img, res, nil, recorder)
		return nil
	}
	defer s.registryLimiter.Release(registryRecord)

	res, snapshot := s.checkSingleImageInBatchInternal(ctx, resolvedCreds, img.parts, composeBuildRefs)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if res != nil {
		res.ActivityID = mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()
	}

	s.recordBatchImageCheckInternal(ctx, activityID, img, res, snapshot, recorder)
	return nil
}

func (
	s *ImageUpdateService,
) recordBatchImageCheckInternal(
	ctx context.Context,
	activityID string,
	img batchImage,
	res *imageupdate.Response,
	snapshot *localImageSnapshot,
	recorder *batchImageProgressRecorder,
) {
	progress := recorder.recordInternal(img.refs, res)

	level, message := imageCheckResultMessageInternal(img.canonicalRef, res)
	s.appendImageUpdateActivityMessageInternal(ctx, activityID, level, message, progress, "Checking image")

	if err := s.saveUpdateResultWithSnapshotInternal(ctx, img.canonicalRef, res, snapshot); err != nil {
		slog.WarnContext(ctx, "Failed to save update result", "imageRef", img.canonicalRef, "error", imageDTO.Neutral(err, "update.check-unavailable", 503).Error())
	}
}

// recordDigestPinnedSkipInternal notes a digest-pinned ref as skipped on the
// activity stream and persists its unchanged result.
func (s *ImageUpdateService) recordDigestPinnedSkipInternal(ctx context.Context, activityID, imageRef string, result *imageupdate.Response, progress int) {
	s.appendImageUpdateActivityMessageInternal(ctx, activityID, activitytypes.MessageLevelInfo, imageRef+" — digest pinned, skipped", progress, "Skipping image update check")
	if err := s.saveUpdateResultWithSnapshotInternal(ctx, imageRef, result, nil); err != nil {
		slog.WarnContext(ctx, "Failed to save digest-pinned update result", "imageRef", imageRef, "error", imageDTO.Neutral(err, "update.check-unavailable", 503).Error())
	}
}

// recordInitialBatchResultsInternal stamps the activity ID on the parse-stage
// results and persists digest-pinned refs as skipped, since they never reach
// the registry check stage.
func (s *ImageUpdateService) recordInitialBatchResultsInternal(ctx context.Context, activityID string, initialResults map[string]*imageupdate.Response) {
	for imageRef, result := range initialResults {
		if result == nil {
			continue
		}
		result.ActivityID = mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()
		if strings.TrimSpace(result.Error) != "" {
			continue
		}
		if _, ok := digest.FromReferenceSuffix(imageRef); !ok {
			continue
		}

		s.recordDigestPinnedSkipInternal(ctx, activityID, imageRef, result, 5)
	}
}
