// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"context"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

// GetRuntimeDetail is a receipt-first local read even when the executor is
// unreachable. The caller resolves existing activity access before this lookup.
func (s *RuntimeService) GetRuntimeDetail(ctx context.Context, ref RuntimeRunReference) (*activitytypes.RuntimeExecutionDetail, error) {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	var detail *activitytypes.RuntimeExecutionDetail
	err := s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		detail, err = runtimeDetailSnapshotTxInternal(tx, row, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// RuntimeOutputRelay belongs to one waiter. Retained snapshots replace windows;
// only unseen sequence entries reach the existing output encoder. A gap emits
// an explicit marker, including when disabled/pruned output leaves an empty window.
type RuntimeOutputRelay struct {
	Seen         uint64
	LossReported bool
}

func (r *RuntimeOutputRelay) Relay(detail *activitytypes.RuntimeExecutionDetail, emit func(activitytypes.OutputLine) error) error {
	if detail == nil || detail.RetainedStartSeq == 0 || emit == nil {
		return activitytypes.ErrRuntimeWire
	}
	marker := func() error {
		return emit(activitytypes.OutputLine{Timestamp: time.Now().UTC(), Stream: "stderr", Level: activitytypes.MessageLevelWarning, Text: "[runtime output truncated]"})
	}
	marked := false
	if detail.OutputTruncated && !r.LossReported {
		if err := marker(); err != nil {
			return err
		}
		r.LossReported = true
		marked = true
	}
	if detail.RetainedStartSeq > r.Seen+1 {
		if !marked {
			if err := marker(); err != nil {
				return err
			}
		}
		r.Seen = detail.RetainedStartSeq - 1
	}
	for _, line := range detail.Output {
		if line.OutputSeq <= r.Seen {
			continue
		}
		if line.OutputSeq > r.Seen+1 {
			if err := marker(); err != nil {
				return err
			}
		}
		if err := emit(line.OutputLine); err != nil {
			return err
		}
		r.Seen = line.OutputSeq
	}
	return nil
}

// Wait is a bounded legacy viewer, never an executor. Disconnect only detaches
// this waiter; accepted workers and application-owned reconciliation survive.
func (c *RuntimeRemoteCoordinator) Wait(ctx context.Context, ref RuntimeRunReference, emit func(activitytypes.OutputLine) error) (activitytypes.RuntimeSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if err := c.WaitDispatch(ctx, ref.RunID); err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	relay := &RuntimeOutputRelay{}
	for {
		detail, err := c.service.GetRuntimeDetail(ctx, ref)
		if err != nil {
			return activitytypes.RuntimeSnapshot{}, err
		}
		if emit != nil {
			if err := relay.Relay(detail, emit); err != nil {
				return detail.Activity, err
			}
		}
		if isTerminalActivityStatusInternal(detail.Activity.Status) || detail.Activity.OutcomeState == activitytypes.RuntimeOutcomeUnknown {
			return detail.Activity, nil
		}
		if err := c.schedule.Wait(ctx, 2*time.Second); err != nil {
			return detail.Activity, err
		}
		if _, err := c.Refresh(ctx, ref); err != nil {
			return detail.Activity, err
		}
	}
}
