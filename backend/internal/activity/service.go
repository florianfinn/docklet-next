// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; removed SQLite; coded run projection, guarded updates and bounded output persistence.

package activity

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/samber/mo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
)

const ownerWriteTimeout = 10 * time.Second

func ownerWriteContextInternal(ctx context.Context) (context.Context, context.CancelFunc) {
	if errors.Is(context.Cause(ctx), activitylib.ErrCanceled) {
		return context.WithTimeout(context.WithoutCancel(ctx), ownerWriteTimeout)
	}
	return ctx, func() {}
}

func gatedWriterAllowedInternal(ctx context.Context, model *Activity) bool {
	if model.GateScope == "" {
		return true
	}
	if allowed, _ := ctx.Value(startupRepairContextKey{}).(bool); allowed {
		return true
	}
	id, _ := ctx.Value(gateRunContextKey{}).(string)
	return id == model.ID
}

const (
	defaultActivityHistoryLimit = 100
	defaultActivityMessages     = 500
	// abandonedActivityGrace is how old an untracked queued/running activity
	// must be before the periodic sweep fails it. It covers the window between
	// StartActivity's row creation and the worker's Track call.
	abandonedActivityGrace = 2 * time.Minute

	// terminalPublishRetention bounds how long a terminal publish suppresses
	// stale non-terminal snapshots for the same activity. Stale publishers are
	// goroutines already past their commit, so they publish within moments; the
	// retention only has to outlive that gap while keeping the latch map small.
	terminalPublishRetention = 10 * time.Minute

	// ErrActivityNotCancelable indicates the activity has already reached a terminal
	// state and can no longer be cancelled.

	// subscriberMessageQueueLimit bounds the per-subscriber backlog of "message"
	// events; the oldest message is dropped (and flagged as missed) on overflow.
	subscriberMessageQueueLimit = 256

	cancelledMessageInternal = "Cancelled by user"

	deleteActivitiesBatchSize = 500
)

var ErrActivityNotCancelable = errors.New("activity is not cancelable")
var ErrRunNotOwned = errors.New("gated activity is not owned by this worker")

type gateRunContextKey struct{}
type startupRepairContextKey struct{}

type ActivityService struct {
	db *database.DB

	subscribersMu sync.RWMutex
	subscribers   map[int]*activitySubscriber
	nextSubID     int

	// running maps an active activity ID to the cancel function of its work
	// context, so cancellation requests can interrupt in-flight work. Entries
	// are added by Track and removed when the activity is completed.
	runningMu sync.Mutex
	running   map[string]context.CancelCauseFunc
	// cancelling holds the local gate conflict until completion returns.
	cancelling map[string]bool
	clock      func() time.Time

	// limiter bounds concurrent queue-opted activities per environment.
	// slotReleases maps an activity ID to the release func of the slot it
	// holds; the slot is freed when the activity completes.
	limiter      *activitySlotLimiter
	slotMu       sync.Mutex
	slotReleases map[string]func()

	// terminalPublished records when each activity's terminal snapshot was
	// published, so later non-terminal snapshots from slower goroutines can
	// be dropped (see admitActivityPublishInternal). Entries are pruned on
	// terminal publishes once older than terminalPublishRetention.
	terminalPublishedMu sync.Mutex
	terminalPublished   map[string]time.Time

	// publishLocks serializes the commit→publish window of the non-terminal
	// snapshot writers (AppendMessages, UpdateActivity) per activity, so their
	// events reach subscribers in commit order — without it, a batch that
	// commits first but publishes second would revert progress/step/status
	// fields a concurrent update already streamed. Terminal snapshots are
	// ordered separately by admitActivityPublishInternal. Striped by activity
	// ID hash so no per-activity lifecycle management is needed.
	publishLocks [64]sync.Mutex
}

// activitySubscriber buffers stream events between publishers and one stream
// consumer. "activity" events are coalesced in place per activity ID (only the
// latest pending state matters to the UI), so bulk operations emitting rapid
// progress updates cannot overflow the subscriber and force full-snapshot
// resends. Other events keep arrival order in a FIFO bounded by
// subscriberMessageQueueLimit with drop-oldest on overflow.
type activitySubscriber struct {
	environmentID string
	ch            chan activity.StreamEvent
	done          chan struct{}
	wake          chan struct{}

	mu              sync.Mutex
	missed          bool
	queue           []*pendingStreamEvent
	pendingActivity map[string]*pendingStreamEvent
	messageCount    int
}

type pendingStreamEvent struct {
	event activity.StreamEvent
}

func newActivitySubscriberInternal(environmentID string, ch chan activity.StreamEvent) *activitySubscriber {
	return &activitySubscriber{
		environmentID:   environmentID,
		ch:              ch,
		done:            make(chan struct{}),
		wake:            make(chan struct{}, 1),
		pendingActivity: map[string]*pendingStreamEvent{},
	}
}

func isCoalescableEventInternal(event activity.StreamEvent) bool {
	return event.Type == "activity" && event.ActivityID != ""
}

func (sub *activitySubscriber) enqueue(event activity.StreamEvent) {
	sub.mu.Lock()
	if isCoalescableEventInternal(event) {
		if pending, ok := sub.pendingActivity[event.ActivityID]; ok {
			pending.event = event
			sub.mu.Unlock()
			return
		}
	} else {
		if sub.messageCount >= subscriberMessageQueueLimit {
			sub.dropOldestMessageLockedInternal()
		}
		sub.messageCount++
	}
	entry := &pendingStreamEvent{event: event}
	sub.queue = append(sub.queue, entry)
	if isCoalescableEventInternal(event) {
		sub.pendingActivity[event.ActivityID] = entry
	}
	sub.mu.Unlock()

	select {
	case sub.wake <- struct{}{}:
	default:
	}
}

func (sub *activitySubscriber) dropOldestMessageLockedInternal() {
	for i, entry := range sub.queue {
		if isCoalescableEventInternal(entry.event) {
			continue
		}
		sub.queue = append(sub.queue[:i], sub.queue[i+1:]...)
		sub.messageCount--
		sub.missed = true
		ctx := context.Background() //nolint:forbidigo // Subscriber buffer trimming runs outside any request context.
		slog.WarnContext(ctx, "activity subscriber message buffer full; snapshot will be sent on next heartbeat",
			"environmentId", sub.environmentID)
		return
	}
}

func (sub *activitySubscriber) nextInternal() mo.Option[activity.StreamEvent] {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	if len(sub.queue) == 0 {
		return mo.None[activity.StreamEvent]()
	}
	entry := sub.queue[0]
	sub.queue = sub.queue[1:]
	event := entry.event
	if isCoalescableEventInternal(event) {
		if sub.pendingActivity[event.ActivityID] == entry {
			delete(sub.pendingActivity, event.ActivityID)
		}
	} else {
		sub.messageCount--
	}
	return mo.Some(event)
}

func (sub *activitySubscriber) pump() {
	defer close(sub.ch)
	for {
		event, ok := sub.nextInternal().Get()
		if !ok {
			select {
			case <-sub.wake:
				continue
			case <-sub.done:
				return
			}
		}
		select {
		case sub.ch <- event:
		case <-sub.done:
			return
		}
	}
}

type (
	StartActivityRequest         = activitylib.StartRequest
	UpdateActivityRequest        = activitylib.UpdateRequest
	AppendActivityMessageRequest = activitylib.AppendMessageRequest
)

func NewActivityService(db *database.DB, settingsService *settings.SettingsService) *ActivityService {
	return &ActivityService{
		db:                db,
		subscribers:       map[int]*activitySubscriber{},
		running:           map[string]context.CancelCauseFunc{},
		limiter:           newActivitySlotLimiterInternal(settingsService),
		slotReleases:      map[string]func(){},
		terminalPublished: map[string]time.Time{},
	}
}

// Track derives a cancelable work context bound to activityID and registers its
// cancel function so RequestCancel can interrupt the work. The registration is
// released when the activity is completed (see CompleteActivity) or when the
// returned context is otherwise no longer needed. Implements activitylib.Tracker.
func (s *ActivityService) Track(ctx context.Context, activityID string) context.Context {
	activityID = strings.TrimSpace(activityID)
	if s == nil || activityID == "" {
		return ctx
	}
	workCtx, cancel := context.WithCancelCause(ctx)
	s.runningMu.Lock()
	if s.running == nil {
		s.running = map[string]context.CancelCauseFunc{}
	}
	if existing, ok := s.running[activityID]; ok {
		// Replace any stale registration to avoid leaking the prior context.
		existing(nil)
	}
	s.running[activityID] = cancel
	delete(s.cancelling, activityID)
	s.runningMu.Unlock()
	return workCtx
}

// RequestCancel cancels the work context registered for activityID, signalling
// activitylib.ErrCanceled as the cause. It returns whether a running activity
// was found in this process.
func (s *ActivityService) RequestCancel(activityID string) bool {
	return s.cancelTrackedInternal(activityID, activitylib.ErrCanceled)
}

func (s *ActivityService) cancelTrackedInternal(activityID string, cause error) bool {
	activityID = strings.TrimSpace(activityID)
	if s == nil || activityID == "" {
		return false
	}

	s.runningMu.Lock()
	cancel, ok := s.running[activityID]
	if ok {
		if s.cancelling == nil {
			s.cancelling = make(map[string]bool)
		}
		s.cancelling[activityID] = true
	}
	s.runningMu.Unlock()
	if !ok {
		return false
	}
	cancel(cause)
	return true
}

// releaseCancelInternal removes and cancels the registration for activityID.
// Cancelling with a nil cause is a no-op if the context was already cancelled
// (the first cause wins), so a prior ErrCanceled cause is preserved.
func (s *ActivityService) releaseCancelInternal(activityID string) {
	activityID = strings.TrimSpace(activityID)
	if s == nil || activityID == "" {
		return
	}

	s.runningMu.Lock()
	cancel, ok := s.running[activityID]
	if ok {
		delete(s.running, activityID)
		delete(s.cancelling, activityID)
	}
	s.runningMu.Unlock()
	if ok {
		cancel(nil)
	}
}

func (s *ActivityService) checkInitInternal() error {
	if s == nil || s.db == nil {
		return errors.New("activity service not initialized")
	}
	return nil
}

func (s *ActivityService) StartActivity(ctx context.Context, req StartActivityRequest) (*activity.Activity, error) {
	if err := s.checkInitInternal(); err != nil {
		return nil, err
	}
	if code := strings.TrimSpace(req.MessageCode); code != "" && !activity.Valid(code) {
		return nil, fmt.Errorf("unknown activity message code: %s", code)
	}

	now := time.Now()
	environmentID := cmp.Or(strings.TrimSpace(req.EnvironmentID), "0")

	var startedByUserID, startedByUsername, startedByDisplayName *string
	if req.StartedBy != nil {
		startedByUserID = mo.EmptyableToOption(strings.TrimSpace(req.StartedBy.ID)).ToPointer()
		startedByUsername = mo.EmptyableToOption(strings.TrimSpace(req.StartedBy.Username)).ToPointer()
		if req.StartedBy.DisplayName != nil {
			startedByDisplayName = mo.EmptyableToOption(strings.TrimSpace(*req.StartedBy.DisplayName)).ToPointer()
		}
	}

	batchID := req.BatchID
	if batchID == nil {
		if contextBatchID := utils.ActivityBatchIDFromContext(ctx); contextBatchID != "" {
			batchID = &contextBatchID
		}
	}

	// Queue-opted activities take a concurrency slot up front when one is
	// free; otherwise they are created as queued and AwaitActivitySlot blocks
	// until a slot opens.
	status := activity.StatusRunning
	var slotRelease func()
	if req.Queue {
		if req.DeferSlot {
			status = activity.StatusQueued
		} else if release, ok := s.limiter.tryAcquireInternal(ctx, environmentID).Get(); ok {
			slotRelease = release
		} else {
			status = activity.StatusQueued
		}
	}

	model := &Activity{
		EnvironmentID:        environmentID,
		BatchID:              copyPtrInternal(batchID),
		Type:                 req.Type,
		Status:               status,
		ResourceType:         copyPtrInternal(req.ResourceType),
		ResourceID:           copyPtrInternal(req.ResourceID),
		ResourceName:         copyPtrInternal(req.ResourceName),
		StartedByUserID:      startedByUserID,
		StartedByUsername:    startedByUsername,
		StartedByDisplayName: startedByDisplayName,
		Progress:             clampProgressPtrInternal(req.Progress),
		Step:                 strings.TrimSpace(req.Step),
		LatestMessage:        strings.TrimSpace(req.LatestMessage),
		MessageCode:          cmp.Or(strings.TrimSpace(req.MessageCode), "activity."+string(status)),
		AffectedServices:     servicesJSONInternal(req.AffectedServices),
		StartedAt:            now,
		Metadata:             cloneJSONInternal(req.Metadata),
		CreatedAt:            now,
	}
	if model.Type == "" {
		model.Type = activity.TypeAutoUpdate
	}

	if err := s.db.WithContext(ctx).Create(model).Error; err != nil {
		if slotRelease != nil {
			slotRelease()
		}
		return nil, fmt.Errorf("failed to create activity: %w", err)
	}
	if slotRelease != nil {
		s.registerSlotReleaseInternal(model.ID, slotRelease)
	}

	dto := activityToDTOInternal(model)
	s.publishActivityInternal(dto)
	return &dto, nil
}

// StartTrackedActivity holds the cancellation lock across persistence and
// registration, so cancellation cannot treat the new row as untracked.
func (s *ActivityService) StartTrackedActivity(ctx context.Context, req StartActivityRequest) (*activity.Activity, context.Context, error) {
	if err := s.checkInitInternal(); err != nil {
		return nil, nil, err
	}
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	started, err := s.StartActivity(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	workCtx, cancel := context.WithCancelCause(ctx)
	if s.running == nil {
		s.running = map[string]context.CancelCauseFunc{}
	}
	s.running[started.ID] = cancel
	delete(s.cancelling, started.ID)
	return started, workCtx, nil
}

func (s *ActivityService) registerSlotReleaseInternal(activityID string, release func()) {
	s.slotMu.Lock()
	if existing, ok := s.slotReleases[activityID]; ok {
		existing()
	}
	s.slotReleases[activityID] = release
	s.slotMu.Unlock()
}

func (s *ActivityService) releaseSlotInternal(activityID string) {
	if s == nil {
		return
	}
	s.slotMu.Lock()
	release, ok := s.slotReleases[activityID]
	if ok {
		delete(s.slotReleases, activityID)
	}
	s.slotMu.Unlock()
	if ok {
		release()
	}
}

// AwaitActivitySlot blocks until the queued activity holds a concurrency slot,
// then flips its status to running. It returns immediately when the activity
// already took a slot at creation. On cancellation the context cause is
// returned and the activity stays queued for its caller to finalize.
// Implements activitylib.SlotWaiter.
func (s *ActivityService) AwaitActivitySlot(ctx context.Context, activityID, environmentID string) error {
	if err := s.checkInitInternal(); err != nil {
		return err
	}
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return errors.New("activity id is required")
	}
	environmentID = cmp.Or(strings.TrimSpace(environmentID), "0")

	s.slotMu.Lock()
	_, held := s.slotReleases[activityID]
	s.slotMu.Unlock()
	if held {
		if err := ctx.Err(); err != nil {
			return context.Cause(ctx)
		}
		return nil
	}

	release, err := s.limiter.acquireInternal(ctx, environmentID)
	if err != nil {
		return err
	}
	s.registerSlotReleaseInternal(activityID, release)

	if _, updateErr := s.UpdateActivity(ctx, activityID, UpdateActivityRequest{Status: activity.StatusRunning}); updateErr != nil {
		s.releaseSlotInternal(activityID)
		return updateErr
	}
	if err := ctx.Err(); err != nil {
		s.releaseSlotInternal(activityID)
		return context.Cause(ctx)
	}
	return nil
}

// AwaitActivitySlotBounded waits for a concurrency slot like AwaitActivitySlot
// but gives up after timeouts.DefaultActivitySlotWait, returning
// ActivitySlotWaitTimeoutError so the caller fails the queued activity loudly
// instead of parking forever behind long-running slot holders.
func (s *ActivityService) AwaitActivitySlotBounded(ctx context.Context, activityID, environmentID string) error {
	slotCtx, cancel := context.WithTimeout(ctx, timeouts.DefaultActivitySlotWait)
	defer cancel()
	if err := s.AwaitActivitySlot(slotCtx, activityID, environmentID); err != nil {
		if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			return common.Classify(common.ErrTimeout, errors.New("timed out waiting for a free activity slot; other long-running activities are holding all slots"))
		}
		return err
	}
	return nil
}

func (s *ActivityService) UpdateActivity(ctx context.Context, activityID string, req UpdateActivityRequest) (*activity.Activity, error) {
	if err := s.checkInitInternal(); err != nil {
		return nil, err
	}
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return nil, errors.New("activity id is required")
	}
	if isTerminalActivityStatusInternal(req.Status) {
		return nil, errors.New("terminal activity status requires CompleteActivity")
	}
	ctx, cancel := ownerWriteContextInternal(ctx)
	defer cancel()

	updates := map[string]any{
		"updated_at": gorm.Expr("NOW()"),
	}
	if req.Status != "" {
		updates["status"] = req.Status
		if req.MessageCode == nil {
			updates["message_code"] = activityCodeInternal(req.Status)
		}
	}
	if req.ResourceName != nil {
		updates["resource_name"] = strings.TrimSpace(*req.ResourceName)
	}
	if req.Progress != nil {
		updates["progress"] = *clampProgressPtrInternal(req.Progress)
	}
	if req.Step != nil {
		updates["step"] = strings.TrimSpace(*req.Step)
	}
	if req.LatestMessage != nil {
		updates["latest_message"] = strings.TrimSpace(*req.LatestMessage)
	}
	if req.MessageCode != nil {
		code := strings.TrimSpace(*req.MessageCode)
		if !activity.Valid(code) {
			return nil, fmt.Errorf("unknown activity message code: %s", code)
		}
		updates["message_code"] = code
	}
	if req.AffectedServices != nil {
		updates["affected_services"] = servicesJSONInternal(req.AffectedServices)
	}
	if req.Error != nil {
		updates["error"] = strings.TrimSpace(*req.Error)
	}
	if req.Metadata != nil {
		updates["metadata"] = cloneJSONInternal(req.Metadata)
	}

	// Hold the per-activity publish lock across commit and publish so this
	// snapshot cannot be overtaken on the stream by a concurrent append batch
	// that committed earlier but would publish later.
	lock := s.publishLockInternal(activityID)
	lock.Lock()
	defer lock.Unlock()

	var model Activity
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Activity
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", activityID).Error; err != nil {
			return err
		}
		if isTerminalActivityStatusInternal(current.Status) {
			return ErrActivityNotCancelable
		}
		if !gatedWriterAllowedInternal(ctx, &current) {
			return ErrRunNotOwned
		}
		query := tx.Model(&Activity{}).Where("id = ? AND status NOT IN ?", activityID, terminalActivityStatusesInternal())
		result := query.Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("failed to update activity: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errors.New("activity not found or completed")
		}
		if err := tx.First(&model, "id = ?", activityID).Error; err != nil {
			return fmt.Errorf("failed to load updated activity: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	dto := activityToDTOInternal(&model)
	s.publishActivityInternal(dto)
	return &dto, nil
}

func (s *ActivityService) AppendMessage(ctx context.Context, activityID string, req AppendActivityMessageRequest) (*activity.Message, error) {
	messages, err := s.AppendMessages(ctx, activityID, []AppendActivityMessageRequest{req})
	if err != nil || len(messages) == 0 {
		return nil, err
	}
	return &messages[0], nil
}

// buildAppendBatchInternal turns the request batch into insertable message rows
// plus the coalesced Activity column updates (latest message/progress/step from
// the last request carrying each).
func buildAppendBatchInternal(activityID string, reqs []AppendActivityMessageRequest, now time.Time) ([]*ActivityMessage, map[string]any) {
	messages := make([]*ActivityMessage, 0, len(reqs))
	latestMessage := ""
	latestCode := ""
	var latestProgress *int
	latestStep := ""
	for _, req := range reqs {
		messageText := strings.TrimSpace(req.Message)
		if messageText == "" {
			continue
		}
		if len(messageText) > 8192 {
			messageText = messageText[:8192]
		}

		level := cmp.Or(req.Level, activity.MessageLevelInfo)
		code := cmp.Or(strings.TrimSpace(req.Code), "activity.message")

		messages = append(messages, &ActivityMessage{
			ActivityID: activityID,
			Level:      level,
			Message:    messageText,
			Code:       code,
			Step:       strings.TrimSpace(req.Step),
			Payload:    cloneJSONInternal(req.Payload),
			// Spread inside the shared timestamp so the created_at sort
			// used for retrieval keeps the original line order.
			CreatedAt: now.Add(time.Duration(len(messages)) * time.Microsecond),
		})

		latestMessage = messageText
		latestCode = code
		if req.Progress != nil {
			latestProgress = req.Progress
		}
		if step := strings.TrimSpace(req.Step); step != "" {
			latestStep = step
		}
	}
	if len(messages) == 0 {
		return nil, nil
	}

	updates := map[string]any{
		"latest_message": latestMessage,
		"message_code":   latestCode,
		"updated_at":     gorm.Expr("NOW()"),
	}
	if latestProgress != nil {
		updates["progress"] = *clampProgressPtrInternal(latestProgress)
	}
	if latestStep != "" {
		updates["step"] = latestStep
	}
	return messages, updates
}

// appendBatchWithRetryInternal retries transient write contention because the
// Writer drain cannot replay a failed message and output batch.
func (s *ActivityService) appendBatchWithRetryInternal(ctx context.Context, activityID string, messages []*ActivityMessage, updates map[string]any, output []activity.OutputLine) (*Activity, error) {
	var current Activity
	const appendWriteAttempts = 3
	var writeErr error
	for attempt := 1; attempt <= appendWriteAttempts; attempt++ {
		writeErr = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", activityID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("activity not found")
				}
				return fmt.Errorf("failed to load activity: %w", err)
			}
			if !gatedWriterAllowedInternal(ctx, &current) {
				return ErrRunNotOwned
			}
			if isTerminalActivityStatusInternal(current.Status) {
				return ErrActivityNotCancelable
			}
			if current.GateScope != "" {
				var databaseNow time.Time
				if err := tx.Raw("SELECT NOW()").Scan(&databaseNow).Error; err != nil {
					return err
				}
				for i := range messages {
					messages[i].CreatedAt = databaseNow.Add(time.Duration(i) * time.Microsecond)
				}
			}
			if len(output) > 0 {
				lines, truncated := appendOutputLinesInternal(current.Output, output)
				updates["output"] = lines
				updates["output_truncated"] = current.OutputTruncated || truncated
			}

			query := tx.Model(&Activity{}).Where("id = ? AND status NOT IN ?", activityID, terminalActivityStatusesInternal())
			result := query.Updates(updates)
			if result.Error != nil {
				return fmt.Errorf("failed to update activity latest message: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				return ErrActivityNotCancelable
			}
			if len(messages) > 0 {
				if err := tx.Create(&messages).Error; err != nil {
					return fmt.Errorf("failed to append activity message: %w", err)
				}
			}
			if err := tx.First(&current, "id = ?", activityID).Error; err != nil {
				return fmt.Errorf("failed to load updated activity: %w", err)
			}
			return nil
		})
		if writeErr == nil {
			return &current, nil
		}
		if attempt == appendWriteAttempts || !dbutil.IsRetryableWriteError(writeErr) {
			return nil, writeErr
		}
		slog.WarnContext(ctx, "retrying activity message batch write", "activityId", activityID, "attempt", attempt, "error", writeErr)
		time.Sleep(250 * time.Millisecond * time.Duration(attempt))
	}
	return nil, writeErr
}

// PatchActivityMetadata merges patch into the activity's existing metadata,
// unlike UpdateActivity which replaces the metadata wholesale.
func (s *ActivityService) PatchActivityMetadata(ctx context.Context, activityID string, patch database.JSON) error {
	if err := s.checkInitInternal(); err != nil {
		return err
	}
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return errors.New("activity id is required")
	}
	if len(patch) == 0 {
		return nil
	}
	ctx, cancel := ownerWriteContextInternal(ctx)
	defer cancel()

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record Activity
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "id = ?", activityID).Error; err != nil {
			return fmt.Errorf("failed to load activity: %w", err)
		}
		if !gatedWriterAllowedInternal(ctx, &record) {
			return ErrRunNotOwned
		}
		if isTerminalActivityStatusInternal(record.Status) {
			return ErrActivityNotCancelable
		}
		merged := cloneJSONInternal(record.Metadata)
		if merged == nil {
			merged = database.JSON{}
		}
		maps.Copy(merged, patch)
		query := tx.Model(&Activity{}).Where("id = ? AND status IN ?", activityID, []activity.Status{activity.StatusQueued, activity.StatusRunning})
		result := query.Updates(map[string]any{"metadata": merged, "updated_at": gorm.Expr("NOW()")})
		if result.Error != nil {
			return fmt.Errorf("failed to patch activity metadata: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrActivityNotCancelable
		}
		return nil
	})
}
