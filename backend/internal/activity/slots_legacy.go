// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extracted legacy slot wait boundary; receipt-owned runtime work is refused; Local module namespace; SQLite persistence; coded run projection, guarded updates and configurable bounded output persistence.

package activity

import (
	"cmp"
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/types/activity"
	"strings"
)

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
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if err := s.refuseRuntimeWriterInternal(ctx, activityID); err != nil {
		return err
	}

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
