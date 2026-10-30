// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Application-owned storage-only settlement retries and compact cleanup; M14 recreate recovery retries.

package activity

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// RunMaintenance belongs to the application lifetime. Each failed settlement
// retains its immutable joined result and slot; this loop never executes work.
func (s *RuntimeService) RunMaintenance(ctx context.Context) error {
	return s.runMaintenanceInternal(ctx, time.Second, time.Hour)
}

// runtimeRecreateRetryEvery paces Docker reads for held, unresolved recreates.
const runtimeRecreateRetryEvery = 30 * time.Second

func (s *RuntimeService) runMaintenanceInternal(ctx context.Context, retryEvery, pruneEvery time.Duration) error {
	if err := s.checkInternal(); err != nil {
		return err
	}
	retry := time.NewTicker(retryEvery)
	recreate := time.NewTicker(runtimeRecreateRetryEvery)
	prune := time.NewTicker(pruneEvery)
	defer retry.Stop()
	defer recreate.Stop()
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-retry.C:
			_ = s.retryRetainedInternal(ctx)
		case <-recreate.C:
			if err := s.RetryRecreateRecovery(ctx); err != nil && ctx.Err() == nil {
				slog.WarnContext(ctx, "Recreate recovery deferred", "error", err)
			}
		case <-prune.C:
			_, _ = s.activity.pruneRuntimeInternal(ctx, runtimePruneCapInternal)
		}
	}
}

func (s *RuntimeService) retryRetainedInternal(ctx context.Context) error {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	if s.storagePaused {
		return nil
	}
	var result error
	for _, owner := range s.owners {
		if ctx.Err() != nil {
			return errors.Join(result, ctx.Err())
		}
		if owner.result != nil {
			_, owner.err = s.settleOwnerInternal(owner)
			result = errors.Join(result, owner.err)
		}
	}
	return result
}

// JoinLocal waits for actual workflow/output barriers after application stop.
// A caller timeout leaves durable exclusion; it is never terminal evidence.
func (s *RuntimeService) JoinLocal(ctx context.Context) error {
	s.activity.runningMu.Lock()
	owners := make([]*runtimeOwnerInternal, 0, len(s.owners))
	for _, owner := range s.owners {
		owners = append(owners, owner)
	}
	s.activity.runningMu.Unlock()
	for _, owner := range owners {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-owner.attemptDone:
		}
	}
	return s.retryRetainedInternal(ctx)
}

func (s *RuntimeService) PruneRuntime(ctx context.Context) error {
	_, err := s.activity.pruneRuntimeInternal(ctx, runtimePruneCapInternal)
	return err
}
