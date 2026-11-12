// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Runtime service and fresh identity port with single-owner production construction.

package activity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	containertypes "github.com/florianfinn/docklet-next/types/container"
	"gorm.io/gorm"
)

// RuntimeResolvedTarget is current identity evidence, not inventory cache data.
// ComposeManaged without a persisted parent is an admission refusal.
type RuntimeResolvedTarget struct {
	Origin              *containertypes.OriginFields
	ComposeProjectLabel bool
	Protected           bool
	Target              RuntimeTarget
	ComposeManaged      bool
}

type RuntimeTargetResolver interface {
	ResolveRuntimeTarget(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error)
}

// RuntimeService shares the existing ActivityService mutex, pool and clock.
// DI constructs one owner per ActivityService. Local workers register inside
// the final admission mutex/commit boundary.
type RuntimeService struct {
	restoreMu      sync.Mutex
	intakePaused   bool
	storagePaused  bool
	remote         *RuntimeRemoteCoordinator
	receiver       *RuntimeReceiver
	activity       *ActivityService
	resolver       RuntimeTargetResolver
	localExecution *RuntimeLocalConfig
	owners         map[string]*runtimeOwnerInternal
	configErr      error
	outputAppender activitylib.MessageAppender
}

func NewRuntimeService(activity *ActivityService, resolver RuntimeTargetResolver, local ...RuntimeLocalConfig) *RuntimeService {
	s := &RuntimeService{activity: activity, resolver: resolver, outputAppender: activity, owners: make(map[string]*runtimeOwnerInternal)}
	if len(local) > 1 {
		s.configErr = errRuntimeInvalidInternal
	}
	if len(local) == 1 {
		config := local[0]
		s.localExecution = &config
		if config.ApplicationContext == nil || config.Executor == nil || config.Store.EnvironmentID != "0" ||
			config.Store.StoreGeneration == "" || config.Store.ExecutorGeneration != config.Store.StoreGeneration ||
			config.Store.ManagerID == "" || config.Store.RegistrationID == "" {
			s.configErr = errRuntimeInvalidInternal
		}
	}
	return s
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
	if errors.As(err, &mismatch) || errors.As(err, &gate) || errors.Is(err, errRuntimeInvalidInternal) || errors.Is(err, ErrRuntimeTargetRemoved) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &RuntimeStoreError{Cause: err}
}

func (s *RuntimeService) checkInternal() error {
	if s == nil || s.activity == nil {
		return &RuntimeStoreError{Cause: errors.New("runtime activity store unavailable")}
	}
	if s.configErr != nil {
		return s.configErr
	}
	if err := s.activity.checkInitInternal(); err != nil {
		return &RuntimeStoreError{Cause: err}
	}
	return nil
}

// InitializeGeneration is explicit fresh-store setup, never an Admit fallback.
// Application startup and restore finish generation/continuity repair before readiness.
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

// ConfigureLocal completes the single production instance before readiness.
// Generation initialization must not create a temporary second RuntimeService.
func (s *RuntimeService) ConfigureLocal(config RuntimeLocalConfig) error {
	if err := s.checkInternal(); err != nil {
		return err
	}
	if config.ApplicationContext == nil || config.Executor == nil || config.Store.EnvironmentID != "0" || config.Store.StoreGeneration == "" || config.Store.ExecutorGeneration != config.Store.StoreGeneration || config.Store.ManagerID == "" || config.Store.RegistrationID == "" {
		return errRuntimeInvalidInternal
	}
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	if s.localExecution != nil || len(s.owners) != 0 {
		return errRuntimeInvalidInternal
	}
	s.localExecution = &config
	return nil
}

type runtimeEnvironmentContextKey struct{}
type RuntimeResolverFunc func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error)

func (fn RuntimeResolverFunc) ResolveRuntimeTarget(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
	return fn(ctx, target)
}
func RuntimeEnvironmentFromContext(ctx context.Context) string {
	environment, _ := ctx.Value(runtimeEnvironmentContextKey{}).(string)
	if environment == "" {
		return "0"
	}
	return environment
}

func (s *RuntimeService) LocalStore() RuntimeStoreBinding {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	if s.localExecution == nil {
		return RuntimeStoreBinding{}
	}
	return s.localExecution.Store
}
