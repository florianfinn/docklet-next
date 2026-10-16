// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original restore admission helpers for issue #32.

package activity

import (
	"context"
	"strings"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
)

// RequestCancel cancels the work context registered for activityID, signalling
// activitylib.ErrCanceled as the cause. It returns whether a running activity
// was found in this process.
func (s *ActivityService) RequestCancel(activityID string) bool {
	return s.cancelTrackedInternal(activityID, activitylib.ErrCanceled)
}

// CancelAndDrain waits for local activity workers to release their tracked
// contexts. The restore activity itself stays admitted throughout maintenance.
func (s *ActivityService) CancelAndDrain(ctx context.Context, exceptID string) error {
	if s == nil {
		return nil
	}
	s.runningMu.Lock()
	for id, cancel := range s.running {
		if id != exceptID {
			cancel(activitylib.ErrCanceled)
		}
	}
	s.runningMu.Unlock()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.runningMu.Lock()
		active := false
		for id := range s.running {
			if id != exceptID {
				active = true
				break
			}
		}
		s.runningMu.Unlock()
		if !active {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *ActivityService) cancelTrackedInternal(activityID string, cause error) bool {
	activityID = strings.TrimSpace(activityID)
	if s == nil || activityID == "" {
		return false
	}

	s.runningMu.Lock()
	if s.sealed[activityID] {
		s.runningMu.Unlock()
		return false
	}
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

// SealCancellation marks the point after which cancelling a restore cannot
// imply that its file replacement was reversed.
func (s *ActivityService) SealCancellation(activityID string) error {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	if s.cancelling[activityID] || s.running[activityID] == nil {
		return ErrActivityNotCancelable
	}
	if s.sealed == nil {
		s.sealed = make(map[string]bool)
	}
	s.sealed[activityID] = true
	return nil
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
		delete(s.sealed, activityID)
	}
	s.runningMu.Unlock()
	if ok {
		cancel(nil)
	}
}
