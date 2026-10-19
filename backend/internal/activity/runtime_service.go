// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant internal service and fresh identity port; no production construction.

package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

// RuntimeResolvedTarget is current identity evidence, not inventory cache data.
// ComposeManaged without a persisted parent is an admission refusal.
type RuntimeResolvedTarget struct {
	Target         RuntimeTarget
	ComposeManaged bool
}

type RuntimeTargetResolver interface {
	ResolveRuntimeTarget(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error)
}

// RuntimeService shares the existing ActivityService mutex, pool and clock.
// Only tests construct it until compatibility activation. B1 commits acceptance
// only; worker registration must later share its final mutex/commit boundary.
type RuntimeService struct {
	activity *ActivityService
	resolver RuntimeTargetResolver
}

func NewRuntimeService(activity *ActivityService, resolver RuntimeTargetResolver) *RuntimeService {
	return &RuntimeService{activity: activity, resolver: resolver}
}

// RuntimeStoreError refuses intake while preserving the driver's or generation
// cause. It must never be interpreted as absent compact evidence.
type RuntimeStoreError struct{ Cause error }

func (e *RuntimeStoreError) Error() string {
	return fmt.Sprintf("runtime store unreadable: %v", e.Cause)
}
func (e *RuntimeStoreError) Unwrap() error { return e.Cause }

type RuntimeTargetError struct {
	Code  string
	Cause error
}

func (e *RuntimeTargetError) Error() string { return e.Code }
func (e *RuntimeTargetError) Unwrap() error { return e.Cause }

func runtimeClassifyStoreInternal(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	// Some drivers return BUSY or a transaction error when a context interrupts
	// BEGIN IMMEDIATE. Preserve cancellation as well as the original driver cause.
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), err)
	}
	var mismatch *runtimeConflictInternal
	var gate *GateConflict
	if errors.As(err, &mismatch) || errors.As(err, &gate) || errors.Is(err, errRuntimeInvalidInternal) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &RuntimeStoreError{Cause: err}
}

func (s *RuntimeService) checkInternal() error {
	if s == nil || s.activity == nil {
		return &RuntimeStoreError{Cause: errors.New("runtime activity store unavailable")}
	}
	if err := s.activity.checkInitInternal(); err != nil {
		return &RuntimeStoreError{Cause: err}
	}
	return nil
}

// InitializeGeneration is explicit fresh-store setup, never an Admit fallback.
// Production readiness and restore/continuity policy remain activation duties.
func (s *RuntimeService) InitializeGeneration(ctx context.Context) (string, error) {
	if err := s.checkInternal(); err != nil {
		return "", err
	}
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	var generation string
	err := s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		var err error
		generation, err = runtimeGenerationTxInternal(tx, now)
		return err
	})
	if err != nil {
		return "", err
	}
	return generation, nil
}

// Caller holds runningMu. Production SQLite opens BEGIN IMMEDIATE, and this
// request context remains attached through lock waits and COMMIT.
func (s *RuntimeService) transactionInternal(ctx context.Context, fn func(*gorm.DB, time.Time) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := runtimeTimeInternal(s.activity.nowInternal())
	err := s.activity.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(tx, now); err != nil {
			return err
		}
		return ctx.Err()
	})
	return runtimeClassifyStoreInternal(ctx, err)
}

type RuntimeAdmissionResult struct {
	activitytypes.RuntimeAdmission
}
