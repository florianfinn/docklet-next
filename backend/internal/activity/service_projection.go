// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-14
// License: BSD-3-Clause
// Deviations: Extracted read, stream and job projection helpers.

package activity

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/user"
	"go.getarcane.app/kit/pkg"
	"gorm.io/gorm"
)

func (s *ActivityService) ListActivitiesPaginated(ctx context.Context, environmentID string, params pagination.QueryParams) ([]activity.Activity, pagination.Response, error) {
	if err := s.checkInitInternal(); err != nil {
		return nil, pagination.Response{}, err
	}

	environmentID = cmp.Or(strings.TrimSpace(environmentID), "0")

	var activities []Activity
	q := s.db.WithContext(ctx).Model(&Activity{}).Where("environment_id = ?", environmentID)
	q = scopeJobActivityVisibilityInternal(ctx, q, environmentID, authz.PermActivitiesRead)

	if term := strings.TrimSpace(params.Search); term != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
		searchPattern := "%" + escaped + "%"
		q = q.Where(
			"type LIKE ? ESCAPE '\\' OR COALESCE(resource_name, '') LIKE ? ESCAPE '\\' OR COALESCE(latest_message, "+
				"'') LIKE ? ESCAPE '\\' OR COALESCE(step, '') LIKE ? ESCAPE '\\' OR COALESCE(error, '') LIKE ? ESCAPE "+
				"'\\'",
			searchPattern, searchPattern, searchPattern, searchPattern, searchPattern,
		)
	}

	q = pagination.ApplyFilter(q, "status", params.Filters["status"])
	q = pagination.ApplyFilter(q, "type", params.Filters["type"])
	q = pagination.ApplyFilter(q, "resource_type", params.Filters["resourceType"])

	if params.Sort == "" {
		// Active rows sort by created_at (immutable) and terminal rows by ended_at
		// (set once), so a row's position only changes on the active->terminal
		// transition instead of on every progress update.
		q = q.Order("CASE WHEN status IN ('queued', 'running') THEN 0 ELSE 1 END ASC").
			Order("COALESCE(ended_at, created_at) DESC").
			Order("id DESC")
	}

	paginationResp, err := pagination.PaginateAndSortDB(params, q, &activities)
	if err != nil {
		return nil, pagination.Response{}, fmt.Errorf("failed to paginate activities: %w", err)
	}

	out := make([]activity.Activity, 0, len(activities))
	for i := range activities {
		out = append(out, activityToDTOInternal(&activities[i]))
	}
	return out, paginationResp, nil
}

func (s *ActivityService) Subscribe(environmentID string) (<-chan activity.StreamEvent, func() bool, func()) {
	ch := make(chan activity.StreamEvent, 64)
	if s == nil {
		close(ch)
		return ch, func() bool { return false }, func() {}
	}

	environmentID = cmp.Or(strings.TrimSpace(environmentID), "0")

	sub := newActivitySubscriberInternal(environmentID, ch)
	s.subscribersMu.Lock()
	s.nextSubID++
	id := s.nextSubID
	s.subscribers[id] = sub
	s.subscribersMu.Unlock()
	go sub.pump()

	missedEvents := func() bool {
		s.subscribersMu.RLock()
		localSub, ok := s.subscribers[id]
		s.subscribersMu.RUnlock()
		if !ok {
			return false
		}

		localSub.mu.Lock()
		defer localSub.mu.Unlock()
		if !localSub.missed {
			return false
		}
		localSub.missed = false
		return true
	}

	unsubscribe := func() {
		s.subscribersMu.Lock()
		localSub, ok := s.subscribers[id]
		if ok {
			delete(s.subscribers, id)
		}
		s.subscribersMu.Unlock()
		if ok {
			// The pump goroutine owns ch and closes it on shutdown.
			close(localSub.done)
		}
	}

	return ch, missedEvents, unsubscribe
}

func (s *ActivityService) publishActivityInternal(item activity.Activity) {
	if !s.admitActivityPublishInternal(item) {
		return
	}
	s.publishInternal(item.EnvironmentID, activity.StreamEvent{
		Type:       "activity",
		ActivityID: item.ID,
		Activity:   &item,
		Timestamp:  time.Now(),
	})
}

func (s *ActivityService) publishLockInternal(activityID string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(activityID))
	return &s.publishLocks[h.Sum32()%uint32(len(s.publishLocks))]
}

func isTerminalActivityStatusInternal(status activity.Status) bool {
	return status == activity.StatusSuccess ||
		status == activity.StatusFailed ||
		status == activity.StatusCancelled
}

// admitActivityPublishInternal orders activity events across the commit →
// publish gap. Writes publish their snapshot after committing, so a goroutine
// that committed before a terminal write can publish after it; since a
// terminal activity emits no further events to correct the stream (and
// subscriber coalescing would even replace an undelivered terminal event in
// place), such a stale non-terminal snapshot must be dropped, not delivered.
// Activities never leave a terminal status, so once a terminal snapshot is
// published, any later non-terminal snapshot for that ID is stale by
// construction.
func (s *ActivityService) admitActivityPublishInternal(item activity.Activity) bool {
	if s == nil {
		return true
	}
	s.terminalPublishedMu.Lock()
	defer s.terminalPublishedMu.Unlock()
	if !isTerminalActivityStatusInternal(item.Status) {
		_, sealed := s.terminalPublished[item.ID]
		return !sealed
	}
	now := time.Now()
	for id, publishedAt := range s.terminalPublished {
		if now.Sub(publishedAt) > terminalPublishRetention {
			delete(s.terminalPublished, id)
		}
	}
	if s.terminalPublished == nil {
		s.terminalPublished = map[string]time.Time{}
	}
	s.terminalPublished[item.ID] = now
	return true
}

func (s *ActivityService) publishMessageInternal(environmentID string, activityType activity.Type, message activity.Message) {
	s.publishInternal(environmentID, activity.StreamEvent{
		Type:         "message",
		ActivityID:   message.ActivityID,
		ActivityType: activityType,
		Message:      &message,
		Timestamp:    time.Now(),
	})
}

func (s *ActivityService) publishInternal(environmentID string, event activity.StreamEvent) {
	if s == nil {
		return
	}
	s.subscribersMu.RLock()
	subs := make([]*activitySubscriber, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		if sub.environmentID == environmentID {
			subs = append(subs, sub)
		}
	}
	s.subscribersMu.RUnlock()

	for _, sub := range subs {
		// Source labeling mutates activity fields independently for each subscriber.
		subscriberEvent := event
		if event.Activity != nil {
			subscriberEvent.Activity = new(*event.Activity)
		}
		subscriberEvent.Activities = slices.Clone(event.Activities)
		sub.enqueue(subscriberEvent)
	}
}

func activityToDTOInternal(model *Activity) activity.Activity {
	if model == nil {
		return activity.Activity{}
	}
	return activity.Activity{
		ID:                  model.ID,
		EnvironmentID:       model.EnvironmentID,
		SourceEnvironmentID: model.EnvironmentID,
		BatchID:             copyPtrInternal(model.BatchID),
		Type:                model.Type,
		Status:              model.Status,
		ResourceType:        copyPtrInternal(model.ResourceType),
		ResourceID:          copyPtrInternal(model.ResourceID),
		ResourceName:        copyPtrInternal(model.ResourceName),
		Progress:            clampProgressPtrInternal(model.Progress),
		Step:                model.Step,
		LatestMessage:       model.LatestMessage,
		MessageCode:         model.MessageCode,
		AffectedServices:    servicesFromJSONInternal(model.AffectedServices),
		StartedBy:           activityStartedByDTOInternal(model),
		StartedAt:           model.StartedAt,
		EndedAt:             copyPtrInternal(model.EndedAt),
		DurationMs:          copyPtrInternal(model.DurationMs),
		Error:               copyPtrInternal(model.Error),
		Metadata:            jsonToMapInternal(model.Metadata),
		CreatedAt:           model.CreatedAt,
		UpdatedAt:           copyPtrInternal(model.UpdatedAt),
	}
}

func activityMessageToDTOInternal(model *ActivityMessage) activity.Message {
	if model == nil {
		return activity.Message{}
	}
	return activity.Message{
		ID:         model.ID,
		ActivityID: model.ActivityID,
		Level:      model.Level,
		Message:    model.Message,
		Code:       model.Code,
		Step:       model.Step,
		Payload:    jsonToMapInternal(model.Payload),
		CreatedAt:  model.CreatedAt,
	}
}

func copyPtrInternal[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}

func clampProgressPtrInternal(value *int) *int {
	if value == nil {
		return nil
	}
	return new(min(max(*value, 0), 100))
}

func cloneJSONInternal(input database.JSON) database.JSON {
	if len(input) == 0 {
		return nil
	}
	out := make(database.JSON, len(input))
	maps.Copy(out, input)
	return out
}

func jsonToMapInternal(input database.JSON) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	maps.Copy(out, input)
	return out
}

func terminalActivityStatusesInternal() []activity.Status {
	return []activity.Status{
		activity.StatusSuccess,
		activity.StatusFailed,
		activity.StatusCancelled,
	}
}

func findTerminalActivityIDsInternal(q *gorm.DB) ([]string, error) {
	var activityIDs []string
	if err := q.Model(&Activity{}).
		Where("status IN ?", terminalActivityStatusesInternal()).
		Pluck("id", &activityIDs).Error; err != nil {
		return nil, err
	}
	return activityIDs, nil
}

func findActivityIDsBeyondHistoryLimitInternal(tx *gorm.DB, maxEntries int) ([]string, error) {
	var activityIDs []string
	if err := tx.Raw(`
		SELECT ranked.id
		FROM (
			SELECT id,
				ROW_NUMBER() OVER (
					PARTITION BY environment_id
					ORDER BY COALESCE(ended_at, updated_at, created_at) DESC, started_at DESC
				) AS activity_rank
			FROM activities
			WHERE status IN ?
		) ranked
		WHERE ranked.activity_rank > ?
	`, terminalActivityStatusesInternal(), maxEntries).Scan(&activityIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to find excess activities: %w", err)
	}
	return activityIDs, nil
}

func deleteActivitiesByIDInternal(tx *gorm.DB, activityIDs []string) (int64, error) {
	if len(activityIDs) == 0 {
		return 0, nil
	}

	var totalDeleted int64
	for i := 0; i < len(activityIDs); i += deleteActivitiesBatchSize {
		end := min(i+deleteActivitiesBatchSize, len(activityIDs))
		batch := activityIDs[i:end]

		if err := tx.Where("activity_id IN ?", batch).Delete(&ActivityMessage{}).Error; err != nil {
			return totalDeleted, fmt.Errorf("failed to delete activity messages: %w", err)
		}
		result := tx.Where("id IN ?", batch).Delete(&Activity{})
		if result.Error != nil {
			return totalDeleted, fmt.Errorf("failed to delete activities: %w", result.Error)
		}
		totalDeleted += result.RowsAffected
	}

	return totalDeleted, nil
}

func activityStartedByDTOInternal(model *Activity) *activity.StartedBy {
	if model.StartedByUsername == nil || strings.TrimSpace(*model.StartedByUsername) == "" {
		return &activity.StartedBy{Username: "System"}
	}

	startedBy := &activity.StartedBy{
		Username: strings.TrimSpace(*model.StartedByUsername),
	}
	if model.StartedByUserID != nil {
		startedBy.UserID = strings.TrimSpace(*model.StartedByUserID)
	}
	if model.StartedByDisplayName != nil {
		startedBy.DisplayName = strings.TrimSpace(*model.StartedByDisplayName)
	}
	return startedBy
}

// FailInterruptedBackups finalizes backup activities before startup admits work.
func (s *ActivityService) FailInterruptedBackups(ctx context.Context, protectedIDs ...string) error {
	var pending []Activity
	if err := s.db.WithContext(
		ctx,
	).Where(
		"status IN ?",
		[]activity.Status{
			activity.StatusQueued,
			activity.StatusRunning,
		},
	).Where(
		"id NOT IN ?",
		append(
			protectedIDs,
			"",
		),
	).Find(
		&pending,
	).Error; err != nil {
		return fmt.Errorf("find interrupted backup activities: %w", err)
	}
	const message = "Backup interrupted by Arcane restart"
	var result error
	for _, entry := range pending {
		if s.isTrackedInternal(entry.ID) {
			continue
		}
		switch entry.Metadata["action"] {
		case "create_volume_backup", "create_system_backup", "run_system_volume_backups", "scheduled_volume_backup", "scheduled_system_backup":
			errMessage := message
			_, err := s.CompleteActivity(ctx, entry.ID, activity.StatusFailed, message, &errMessage)
			result = errors.Join(result, err)
		}
	}
	return result
}

// SyncJobRun updates a durable job summary without taking an execution slot.
// Only the queue owns its lifecycle, including reopening it for an explicit retry.
func (s *ActivityService) SyncJobRun(ctx context.Context, run scheduler.Run, name string) error {
	if err := s.checkInitInternal(); err != nil {
		return err
	}
	if run.ActivityID == "" {
		return errors.New("job activity ID is required")
	}
	if run.ID == "" {
		return errors.New("job run ID is required")
	}
	lock := s.publishLockInternal(run.ActivityID)
	lock.Lock()
	defer lock.Unlock()
	model := jobActivityInternal(run, name)
	if !activity.Valid(model.MessageCode) {
		return fmt.Errorf("unknown job activity message code: %s", model.MessageCode)
	}
	changed := false
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var receipt ActivitySyncReceipt
		receiptErr := tx.First(&receipt, "run_id = ?", run.ID).Error
		if receiptErr != nil && !errors.Is(receiptErr, gorm.ErrRecordNotFound) {
			return receiptErr
		}
		if receiptErr == nil && !receipt.UpdatedAt.Before(run.UpdatedAt) {
			var retained Activity
			if err := tx.First(&retained, "id = ?", run.ActivityID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			} else if err != nil {
				return err
			}
		}
		var existing Activity
		err := tx.First(&existing, "id = ?", run.ActivityID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			changed = true
			if err := tx.Create(&model).Error; err != nil {
				return err
			}
			if err := tx.Create(&ActivityMessage{ActivityID: model.ID, Level: activity.MessageLevelInfo, Message: model.LatestMessage, Code: model.MessageCode, Step: model.Step, CreatedAt: run.UpdatedAt}).Error; err != nil {
				return err
			}
			syncedAt := time.Now().UTC()
			return tx.Save(&ActivitySyncReceipt{RunID: run.ID, UpdatedAt: run.UpdatedAt, SyncedAt: &syncedAt}).Error
		}
		if err != nil {
			return err
		}
		if existing.Type != activity.TypeJobRun {
			return errors.New("job activity identity belongs to another operation")
		}
		if existing.GateScope != "" {
			return ErrRunNotOwned
		}
		if stamp, ok := existing.Metadata["runUpdatedAt"].(string); ok {
			previous, parseErr := time.Parse(time.RFC3339Nano, stamp)
			if parseErr == nil && !previous.Before(run.UpdatedAt) {
				if existing.EnvironmentID == model.EnvironmentID {
					return nil
				}
				existing.EnvironmentID = model.EnvironmentID
				model = existing
			}
		}
		changed = true
		if err := tx.Table("activities").Where("id = ?", run.ActivityID).Updates(map[string]any{
			"environment_id": model.EnvironmentID,
			"status":         model.Status, "step": model.Step, "latest_message": model.LatestMessage,
			"message_code": model.MessageCode, "affected_services": model.AffectedServices,
			"ended_at": model.EndedAt, "duration_ms": model.DurationMs, "error": model.Error,
			"metadata": model.Metadata, "updated_at": model.UpdatedAt,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&ActivityMessage{ActivityID: model.ID, Level: activity.MessageLevelInfo, Message: model.LatestMessage, Code: model.MessageCode, Step: model.Step, CreatedAt: run.UpdatedAt}).Error; err != nil {
			return err
		}
		syncedAt := time.Now().UTC()
		return tx.Save(&ActivitySyncReceipt{RunID: run.ID, UpdatedAt: run.UpdatedAt, SyncedAt: &syncedAt}).Error
	}); err != nil {
		return fmt.Errorf("synchronize job activity: %w", err)
	}
	if changed {
		// Ordinary activities cannot reopen. Job summaries can, after an explicit
		// retry; the publish lock and durable timestamp reject stale snapshots.
		s.terminalPublishedMu.Lock()
		delete(s.terminalPublished, model.ID)
		s.terminalPublishedMu.Unlock()
		s.publishActivityInternal(activityToDTOInternal(&model))
	}
	return nil
}

func jobActivityInternal(run scheduler.Run, name string) Activity {
	status := activity.StatusQueued
	switch run.Status {
	case scheduler.Running:
		status = activity.StatusRunning
	case scheduler.Succeeded, scheduler.Skipped:
		status = activity.StatusSuccess
	case scheduler.Failed, scheduler.Partial, scheduler.NeedsAttention:
		status = activity.StatusFailed
	case scheduler.Canceled:
		status = activity.StatusCancelled
	case scheduler.Queued, scheduler.Waiting, scheduler.Retrying:
	}
	message := cmp.Or(run.Outcome.Message, name+": "+string(run.Status))
	model := Activity{
		ID: run.ActivityID, CreatedAt: run.CreatedAt, UpdatedAt: new(run.UpdatedAt),
		EnvironmentID: run.EnvironmentID, BatchID: new(run.ID), Type: activity.TypeJobRun, Status: status,
		ResourceType: new("job"), ResourceID: new(run.JobID), ResourceName: new(name),
		StartedAt: run.CreatedAt, Step: string(run.Status), LatestMessage: message, MessageCode: "job." + string(run.Status),
		Metadata: database.JSON{
			"jobId": run.JobID, "runId": run.ID, "environmentId": run.EnvironmentID,
			"jobStatus": string(run.Status), "trigger": run.Trigger, "attemptCount": run.AttemptCount,
			"runUpdatedAt": run.UpdatedAt.Format(time.RFC3339Nano), "domainActivityId": run.Outcome.ActivityID,
		},
	}
	services := make([]string, 0, len(run.Outcome.Targets))
	for _, target := range run.Outcome.Targets {
		if target.ResourceType == "service" {
			services = append(services, target.ID)
		}
	}
	model.AffectedServices = servicesJSONInternal(services)
	if run.RequestedBy != "" {
		model.StartedByUserID = new(run.RequestedBy)
		model.StartedByUsername = new(run.RequestedBy)
	}
	if run.Resolution != nil {
		model.Metadata["resolution"] = run.Resolution
		if run.Resolution.ResolvedBy != user.SystemUser.Username {
			model.LatestMessage = "Resolved after review"
		}
	}
	if isTerminalActivityStatusInternal(status) {
		model.EndedAt = new(run.UpdatedAt)
		model.DurationMs = new(max(int64(0), run.UpdatedAt.Sub(run.CreatedAt).Milliseconds()))
	}
	if status == activity.StatusFailed {
		model.Error = new(message)
	}
	return model
}

// remoteActivityPageCeiling mirrors the agent's page clamp in PaginateAndSortDB.
const remoteActivityPageCeiling = 100

// normalizeRemoteActivityParamsInternal applies the agent's paging bounds so both
// sources are cut the same way; -1 stays the explicit "show all" sentinel.
func normalizeRemoteActivityParamsInternal(params pagination.QueryParams) pagination.QueryParams {
	params.Start = max(params.Start, 0)
	switch {
	case params.Limit == -1:
	case params.Limit <= 0:
		params.Limit = 20
	case params.Limit > remoteActivityPageCeiling:
		params.Limit = remoteActivityPageCeiling
	}
	return params
}

// collectActivityWindowInternal reads window rows from a source in pages the
// agent accepts, stopping early when the source runs out. -1 reads everything.
func collectActivityWindowInternal(window int, fetchPage func(start, limit int) ([]activity.Activity, int64, error)) ([]activity.Activity, int64, error) {
	if window == -1 {
		return fetchPage(0, -1)
	}
	var rows []activity.Activity
	var total int64
	for len(rows) < window {
		limit := min(remoteActivityPageCeiling, window-len(rows))
		page, pageTotal, err := fetchPage(len(rows), limit)
		if err != nil {
			return nil, 0, err
		}
		rows = append(rows, page...)
		total = pageTotal
		if len(page) < limit {
			break
		}
	}
	return rows, total, nil
}

// ListRemoteActivities combines manager-owned summaries with agent-owned work
// before slicing the page. The remote records must already match the filters
// and be the agent's newest window; remoteTotal is the agent's filtered count.
func (
	s *ActivityService,
) ListRemoteActivities(
	ctx context.Context,
	environmentID string,
	remote []activity.Activity,
	remoteTotal int64,
	params pagination.QueryParams,
) (
	[]activity.Activity,
	pagination.Response,
	error,
) {
	params = normalizeRemoteActivityParamsInternal(params)
	window := kit.Ternary(params.Limit == -1, -1, params.Start+params.Limit)
	activities, localTotal, err := collectActivityWindowInternal(window, func(start, limit int) ([]activity.Activity, int64, error) {
		local := params
		local.Start, local.Limit = start, limit
		rows, page, err := s.ListActivitiesPaginated(ctx, environmentID, local)
		if err != nil {
			return nil, 0, err
		}
		return rows, page.TotalItems, nil
	})
	if err != nil {
		return nil, pagination.Response{}, err
	}
	for _, item := range remote {
		item.EnvironmentID = environmentID
		activities = append(activities, item)
	}
	total := remoteTotal + localTotal
	config := pagination.Config[activity.Activity]{SortBindings: []pagination.SortBinding[activity.Activity]{{
		Fn: func(a, b activity.Activity) int { return compareActivitiesInternal(a, b, params) },
	}}}
	pageParams := params
	pageParams.Order = pagination.SortAsc
	activities = config.OrderAndPaginate(activities, pageParams)
	return activities, pagination.BuildResponse(total, 0, params), nil
}

func compareActivitiesInternal(a, b activity.Activity, params pagination.QueryParams) int {
	if params.Sort == "" {
		aActive := a.Status == activity.StatusQueued || a.Status == activity.StatusRunning
		bActive := b.Status == activity.StatusQueued || b.Status == activity.StatusRunning
		if aActive != bActive {
			return kit.Ternary(aActive, -1, 1)
		}
		aTime, bTime := a.CreatedAt, b.CreatedAt
		if a.EndedAt != nil {
			aTime = *a.EndedAt
		}
		if b.EndedAt != nil {
			bTime = *b.EndedAt
		}
		return cmp.Or(bTime.Compare(aTime), strings.Compare(b.ID, a.ID))
	}

	var result int
	switch params.Sort {
	case "environmentId":
		result = strings.Compare(a.EnvironmentID, b.EnvironmentID)
	case "type":
		result = cmp.Compare(a.Type, b.Type)
	case "status":
		result = cmp.Compare(a.Status, b.Status)
	case "resourceType":
		result = strings.Compare(kit.FromPtr(a.ResourceType), kit.FromPtr(b.ResourceType))
	case "resourceName":
		result = strings.Compare(kit.FromPtr(a.ResourceName), kit.FromPtr(b.ResourceName))
	case "startedAt":
		result = a.StartedAt.Compare(b.StartedAt)
	case "createdAt":
		result = a.CreatedAt.Compare(b.CreatedAt)
	case "updatedAt":
		result = kit.FromPtr(a.UpdatedAt).Compare(kit.FromPtr(b.UpdatedAt))
	case "endedAt":
		result = kit.FromPtr(a.EndedAt).Compare(kit.FromPtr(b.EndedAt))
	case "durationMs":
		result = cmp.Compare(kit.FromPtr(a.DurationMs), kit.FromPtr(b.DurationMs))
	}
	if params.Order == pagination.SortDesc {
		result = -result
	}
	return cmp.Or(result, strings.Compare(a.ID, b.ID))
}
