// Source: getarcaneapp/arcane/backend/internal/imageupdate/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package imageupdate

import (
	"context"
	"errors"
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"log/slog"
	"maps"
	"slices"

	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	"github.com/florianfinn/docklet-next/types/imageupdate"

	"github.com/moby/moby/api/types/container"

	"github.com/moby/moby/client"
	"github.com/samber/mo"

	"go.getarcane.app/updater/refs"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/tags"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"

	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
)

func (s *ImageUpdateService) CheckMultipleImages(ctx context.Context, imageRefs []string, externalCreds []containerregistry.Credential) (results map[string]*imageupdate.Response, err error) {
	startBatch := time.Now()
	results = make(map[string]*imageupdate.Response, len(imageRefs))
	if len(imageRefs) == 0 {
		return results, nil
	}

	activityID := s.startImageUpdateActivityInternal(ctx, fmt.Sprintf("%d images", len(imageRefs)), len(imageRefs))
	ctx = s.activityService.Track(ctx, activityID)

	// A single deferred finalizer owns the terminal activity write so it runs
	// on success, error, and panic alike — any early return that skipped
	// completion would strand the row in running forever. It reads the Track
	// ctx so a user cancellation still records a cancelled status.
	defer func() {
		if panicErr := utils.PanicToError(recover()); panicErr != nil {
			// Don't re-panic: the caller is the long-lived watcher goroutine.
			err = fmt.Errorf("image update check panicked: %w", panicErr)
			slog.ErrorContext(ctx, "image update check panicked", "activityId", activityID, "error", err)
		}
		if err != nil {
			s.completeImageUpdateActivityInternal(ctx, activityID, false, "Image update check failed: "+imageDTO.Neutral(err, "update.check-unavailable", 503).Error())
			return
		}
		successCount, errorCount := countBatchResultOutcomesInternal(imageRefs, results)
		finalMessage := fmt.Sprintf("Image update check completed: %d checked, %d errors", successCount, errorCount)
		s.completeImageUpdateActivityInternal(ctx, activityID, errorCount == 0, finalMessage)
	}()

	if activityID != "" {
		// Bounded slot wait: update-all runs share these slots and can hold
		// them for a long time; parking here forever would wedge every future
		// scan behind the watcher's single-flight gate. On timeout the
		// finalizer above flips the queued row to failed.
		if slotErr := s.activityService.AwaitActivitySlotBounded(ctx, activityID, "0"); slotErr != nil {
			return results, slotErr
		}
	}
	s.appendImageUpdateActivityMessageInternal(ctx, activityID, activitytypes.MessageLevelInfo, fmt.Sprintf("Checking %d image references", len(imageRefs)), 5, "Preparing image update check")
	slog.DebugContext(ctx, "Starting batch image update check", "imageCount", len(imageRefs), "externalCredCount", len(externalCreds))

	regRepos, initialResults, images := s.parseAndGroupImagesInternal(imageRefs)
	maps.Copy(results, initialResults)
	s.recordInitialBatchResultsInternal(ctx, activityID, initialResults)

	// The aggregate scan gets its own deadline: individual registry RPCs are
	// bounded, but the batch as a whole (including registry limiter waits)
	// was not, and a wedged scan holds its activity slot indefinitely.
	scanCtx, cancelScan := context.WithTimeout(ctx, timeouts.DefaultImageUpdateScan)
	defer cancelScan()

	composeBuildRefs, composeErr := s.composeBuildImageRefsInternal(scanCtx)
	if composeErr != nil {
		err = fmt.Errorf("prepare compose build image references: %w", composeErr)
		return results, err
	}

	resolvedCreds := s.resolveBatchCredentialsInternal(scanCtx, externalCreds)

	slog.DebugContext(ctx, "Resolved batch registry credentials", "credentialCount", len(resolvedCreds), "registryCount", len(regRepos))

	recorder := &batchImageProgressRecorder{
		results: results,
		total:   len(images),
	}
	g, groupCtx := errgroup.WithContext(scanCtx)
	g.SetLimit(10) // Limit concurrency

	for _, img := range images {
		g.Go(func() (checkErr error) {
			// x/sync's errgroup does not recover goroutine panics (they crash
			// the process), so convert them to errors here; the deferred
			// finalizer then records the failed terminal status.
			defer func() {
				if panicErr := utils.PanicToError(recover()); panicErr != nil {
					checkErr = fmt.Errorf("image update check panicked: %w", panicErr)
					slog.ErrorContext(groupCtx, "image update check worker panicked", "activityId", activityID, "imageRef", img.canonicalRef, "error", checkErr)
				}
			}()
			return s.checkBatchImageInternal(groupCtx, activityID, resolvedCreds, img, composeBuildRefs, recorder)
		})
	}

	if err = g.Wait(); err != nil {
		if ctx.Err() == nil && errors.Is(scanCtx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("image update check timed out after %s: %w", timeouts.DefaultImageUpdateScan, err)
		}
		slog.ErrorContext(ctx, "Batch check error", "error", err)
		return results, err
	}

	var tagRefs []string
	for _, imageRef := range imageRefs {
		if res := results[imageRef]; res != nil && res.UpdateType != UpdateTypeLocal {
			tagRefs = append(tagRefs, imageRef)
		}
	}
	containerUpdates, tagErr := s.checkContainerTagUpdatesInternal(scanCtx, tagRefs, resolvedCreds)
	if tagErr != nil {
		return results, tagErr
	}
	tags.AttachContainerUpdates(results, containerUpdates)
	successCount, errorCount := countBatchResultOutcomesInternal(imageRefs, results)
	slog.InfoContext(ctx, "Batch image update check completed",
		"totalImages", len(imageRefs),
		"successCount", successCount,
		"errorCount", errorCount,
		"duration", time.Since(startBatch))

	// A failed flush is already logged; the records stay pending for the next one.
	_ = s.SendBatchUpdateNotifications(ctx)

	return results, nil
}

// SendBatchUpdateNotifications delivers pending update notifications. It
// returns an error only when the pending set could not be determined, so a
// caller about to consume the records can stop before their notifications
// are lost; delivery failures leave the records unnotified for the next flush.
func (s *ImageUpdateService) SendBatchUpdateNotifications(ctx context.Context) error {
	if s.notificationService == nil {
		return nil
	}

	// Serialize the query→send→mark sequence so the poll-end flush and the
	// updater consumption-path flush can't both read the same unnotified set
	// and double-send.
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()

	// completeImageUpdateActivityInternal cancels the activity-tracked ctx before
	// we reach here, so detach from that cancellation — otherwise the
	// unnotified-updates query dies with "context canceled" and no notification is
	// ever dispatched. Deliberately not utils.ActivityRuntimeContext: that helper
	// short-circuits (returns ctx unchanged) for app-lifecycle-marked contexts, and
	// every request/scheduler ctx carries that marker via the server BaseContext,
	// making the detach a no-op in production. WithoutCancel keeps all values and
	// the WithTimeout below keeps the flush bounded.
	ctx = context.WithoutCancel(ctx)

	notifCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	unnotifiedUpdates, err := s.GetUnnotifiedUpdates(notifCtx)
	if err == nil && len(unnotifiedUpdates) > 0 {
		// Records for resources that opted out of monitoring since the check, or
		// whose container is gone, stay unnotified: they resurface if monitoring
		// is re-enabled and are otherwise removed by orphan cleanup.
		err = s.filterUnnotifiedByMonitoringInternal(notifCtx, unnotifiedUpdates)
	}
	switch {
	case err != nil:
		slog.WarnContext(ctx, "Failed to get unnotified updates", "error", imageDTO.Neutral(err, "update.check-unavailable", 503).Error())
		return fmt.Errorf("flush pending update notifications: %w", err)
	case len(unnotifiedUpdates) > 0:
		updatesToNotify := make(map[string]*imageupdate.Response)
		imageIDsToMark := make([]string, 0, len(unnotifiedUpdates))

		for imageID, record := range unnotifiedUpdates {
			imageRef := containerUpdateNotificationKeyInternal(record)
			updatesToNotify[imageRef] = &imageupdate.Response{
				HasUpdate:      record.HasUpdate,
				UpdateType:     record.UpdateType,
				CurrentVersion: record.CurrentVersion,
				LatestVersion:  mo.PointerToOption(record.LatestVersion).OrEmpty(),
				CurrentDigest:  mo.PointerToOption(record.CurrentDigest).OrEmpty(),
				LatestDigest:   mo.PointerToOption(record.LatestDigest).OrEmpty(),
				CheckTime:      record.CheckTime,
				ResponseTimeMs: record.ResponseTimeMs,
				Error:          mo.PointerToOption(record.LastError).OrEmpty(),
				AuthMethod:     mo.PointerToOption(record.AuthMethod).OrEmpty(),
				AuthUsername:   mo.PointerToOption(record.AuthUsername).OrEmpty(),
				AuthRegistry:   mo.PointerToOption(record.AuthRegistry).OrEmpty(),
				UsedCredential: record.UsedCredential,
			}
			imageIDsToMark = append(imageIDsToMark, imageID)
		}

		slog.InfoContext(ctx, "Sending notifications for unnotified updates", "count", len(updatesToNotify))

		delivered, notifErr := s.notificationService.SendBatchImageUpdateNotification(notifCtx, updatesToNotify)
		if notifErr != nil {
			slog.WarnContext(ctx, "Failed to send batch update notification", "error", notifErr.Error())
		}
		// Mark notified when at least one provider delivered — a partial provider
		// failure must not make every healthy provider re-send the same updates on
		// the next poll. Failures remain visible in the delivery history.
		if delivered == 0 {
			slog.DebugContext(ctx, "No providers delivered image update notifications; leaving records unnotified", "count", len(imageIDsToMark))
			return nil
		}
		if markErr := s.MarkUpdatesAsNotified(notifCtx, imageIDsToMark); markErr != nil {
			slog.WarnContext(ctx, "Failed to mark updates as notified", "error", markErr.Error())
		}
	default:
		slog.DebugContext(ctx, "No new updates to notify")
	}
	return nil
}

func (s *ImageUpdateService) CheckAllImages(ctx context.Context, limit int, externalCreds []containerregistry.Credential) (map[string]*imageupdate.Response, error) {
	imageRefs, err := s.getAllImageRefsInternal(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get image references: %w", err)
	}

	if len(imageRefs) == 0 {
		return make(map[string]*imageupdate.Response), nil
	}

	results, err := s.CheckMultipleImages(ctx, imageRefs, externalCreds)
	if err != nil {
		return nil, err
	}

	if cleanupOrphanedRecordsErr := s.CleanupOrphanedRecords(ctx); cleanupOrphanedRecordsErr != nil {
		slog.WarnContext(ctx, "failed to cleanup orphaned image update records after check-all", "error", cleanupOrphanedRecordsErr.Error())
	}

	return results, nil
}

// DeleteRecordsForImages removes image-level rows (keyed by id) and
// container/project rows (keyed by image_id) for the given images.
func (s *ImageUpdateService) DeleteRecordsForImages(ctx context.Context, imageIDs []string) error {
	if s == nil || s.db == nil || len(imageIDs) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).Where("id IN ? OR image_id IN ?", imageIDs, imageIDs).Delete(&ImageUpdateRecord{}).Error; err != nil {
		return fmt.Errorf("failed to delete image update records: %w", err)
	}
	return nil
}

func (s *ImageUpdateService) CleanupOrphanedRecords(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	if s.dockerService == nil {
		return errors.New("docker service unavailable")
	}

	dockerImages, err := s.dockerService.ListImages(ctx)
	if err != nil {
		return err
	}
	dockerImageIDs := make([]string, 0, len(dockerImages))
	for _, img := range dockerImages {
		dockerImageIDs = append(dockerImageIDs, img.ID)
	}

	imageScoped := s.db.WithContext(ctx).Where("container_id = ? AND project_id = ?", "", "")
	if len(dockerImageIDs) > 0 {
		imageScoped = imageScoped.Where("id NOT IN ?", dockerImageIDs)
	}
	imageResult := imageScoped.Delete(&ImageUpdateRecord{})
	if imageResult.Error != nil {
		return fmt.Errorf("failed to delete orphaned records: %w", imageResult.Error)
	}

	containers, err := s.dockerService.ListContainers(ctx)
	if err != nil {
		return err
	}
	containerIDs := make([]string, 0, len(containers))
	for _, cnt := range containers {
		containerIDs = append(containerIDs, cnt.ID)
	}
	containerScoped := s.db.WithContext(ctx).Where("container_id <> ?", "")
	if len(containerIDs) > 0 {
		containerScoped = containerScoped.Where("container_id NOT IN ?", containerIDs)
	}
	containerResult := containerScoped.Delete(&ImageUpdateRecord{})
	if containerResult.Error != nil {
		return fmt.Errorf("failed to delete orphaned container records: %w", containerResult.Error)
	}

	if deleted := imageResult.RowsAffected + containerResult.RowsAffected; deleted > 0 {
		slog.InfoContext(ctx, "Cleaned up orphaned image update records", "deletedCount", deleted)
	} else {
		slog.InfoContext(ctx, "No orphaned image update records found")
	}
	return nil
}

func (s *ImageUpdateService) GetUpdateSummary(ctx context.Context) (*imageupdate.Summary, error) {
	if s == nil || s.dockerService == nil {
		return nil, errors.New("docker service unavailable")
	}

	dockerImages, err := s.dockerService.ListImages(ctx)
	if err != nil {
		return nil, err
	}

	liveImageIDs := make([]string, 0, len(dockerImages))
	for _, img := range dockerImages {
		liveImageIDs = append(liveImageIDs, img.ID)
	}

	return s.getUpdateSummaryForImageIDsInternal(ctx, liveImageIDs)
}

func (s *ImageUpdateService) getUpdateSummaryForImageIDsInternal(ctx context.Context, imageIDs []string) (*imageupdate.Summary, error) {
	summary := &imageupdate.Summary{
		TotalImages: len(imageIDs),
	}

	if s.db == nil || len(imageIDs) == 0 {
		return summary, nil
	}

	var aggregate struct {
		ImagesWithUpdates int64 `gorm:"column:images_with_updates"`
		DigestUpdates     int64 `gorm:"column:digest_updates"`
		ErrorsCount       int64 `gorm:"column:errors_count"`
	}
	if err := s.db.WithContext(ctx).
		Model(&ImageUpdateRecord{}).
		Select(`
			COUNT(DISTINCT CASE WHEN has_update THEN COALESCE(NULLIF(image_id, ''),id) END) AS images_with_updates,
			COUNT(DISTINCT CASE WHEN has_update AND update_type = ? THEN COALESCE(NULLIF(image_id, ''),id) END) AS digest_updates,
			COUNT(DISTINCT CASE WHEN last_error IS NOT NULL AND last_error != '' THEN COALESCE(NULLIF(image_id, ''),id) END) AS errors_count
		`, "digest").
		Where("id IN ? OR image_id IN ?", imageIDs, imageIDs).
		Scan(&aggregate).Error; err != nil {
		return nil, err
	}

	summary.ImagesWithUpdates = int(aggregate.ImagesWithUpdates)
	summary.DigestUpdates = int(aggregate.DigestUpdates)
	summary.ErrorsCount = int(aggregate.ErrorsCount)

	return summary, nil
}

func (s *ImageUpdateService) checkContainerTagUpdatesInternal(ctx context.Context, imageRefs []string, credentials []containerregistry.Credential) (map[string]*imageupdate.Response, error) {
	results := map[string]*imageupdate.Response{}
	wanted := map[string]bool{}
	for _, imageRef := range imageRefs {
		if normalized := refs.NormalizeImageUpdateRef(imageRef); normalized != "" {
			wanted[normalized] = true
		}
	}
	if len(wanted) == 0 {
		return results, nil
	}
	dockerClient, err := s.dockerClientInternal(ctx)
	if err != nil {
		return results, err
	}
	apiCtx, cancel := s.dockerAPIContextInternal(ctx)
	listed, err := dockerClient.ContainerList(apiCtx, client.ContainerListOptions{All: true})
	cancel()
	if err != nil {
		return results, fmt.Errorf("list containers for tag update checks: %w", err)
	}
	return s.tags.Check(ctx, listed.Items, wanted, credentials, s.deleteContainerUpdateRecordsInternal, s.saveContainerTagResultInternal)
}

// staleDigestRecordDeleteChunk bounds the IN list used to clear container
// records whose policy switched to digest tracking.
const staleDigestRecordDeleteChunk = 500

// deleteContainerUpdateRecordsInternal clears the stored results of containers
// whose policy no longer tracks tags, in bounded IN-list chunks.
func (s *ImageUpdateService) deleteContainerUpdateRecordsInternal(ctx context.Context, containerIDs []string) error {
	if s.db == nil {
		return nil
	}
	for chunk := range slices.Chunk(containerIDs, staleDigestRecordDeleteChunk) {
		if err := s.db.WithContext(ctx).Where("container_id IN ?", chunk).Delete(&ImageUpdateRecord{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *ImageUpdateService) saveContainerTagResultInternal(ctx context.Context, cnt container.Summary, result *imageupdate.Response) error {
	sanitizeCheckResult(result)
	if s.db == nil {
		return nil
	}
	parsed, err := refs.NormalizeReference(cnt.Image)
	if err != nil {
		return err
	}
	id := "container::" + cnt.ID
	policyKey := imageref.UpdatePolicyKey(cnt.Image, cnt.Labels)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A previous good result must not survive a rate limit under a different policy.
		if deleteStaleUpdatesErr := tx.Where("id = ? AND policy_key <> ?", id, policyKey).Delete(&ImageUpdateRecord{}).Error; deleteStaleUpdatesErr != nil {
			return deleteStaleUpdatesErr
		}
		if savePreparedUpdateResultWithTxErr := savePreparedUpdateResultWithTxInternal(tx, id, parsed.RegistryHost+"/"+parsed.Repository, parsed.Tag, result); savePreparedUpdateResultWithTxErr != nil {
			return savePreparedUpdateResultWithTxErr
		}
		return tx.Model(&ImageUpdateRecord{}).Where("id = ?", id).Updates(map[string]any{"container_id": cnt.ID, "image_id": cnt.ImageID, "policy_key": policyKey}).Error
	})
}

func containerUpdateNotificationKeyInternal(record *ImageUpdateRecord) string {
	imageRef := fmt.Sprintf("%s:%s", record.Repository, record.Tag)
	if record.ContainerID != "" {
		return imageRef + " (container " + record.ContainerID + ")"
	}
	return imageRef
}
