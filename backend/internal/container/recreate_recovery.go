// Source: docklet-next/issues/367
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original bounded startup recreate recovery over transient Docker errors.

package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/containerd/errdefs"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/moby/moby/client"
)

// recreateRecoveryTiming bounds startup recovery: one budget for all journals
// and attempts, a per-inspect budget, and a few passes over pending journals.
var recreateRecoveryTiming = struct {
	budget, call, pause time.Duration
	attempts            int
}{budget: 30 * time.Second, call: 5 * time.Second, pause: 2 * time.Second, attempts: 3}

type recreateCallBudgetKey struct{}

// recreateCallContext applies the recovery per-call budget to one Docker
// inspect; outside startup recovery the caller's context is unchanged.
func recreateCallContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if budget, ok := ctx.Value(recreateCallBudgetKey{}).(time.Duration); ok && budget > 0 {
		return context.WithTimeout(ctx, budget)
	}
	return ctx, func() {}
}

// recreateRecoveryTransient reports a daemon that is unavailable or did not
// answer within the recovery budgets. Journal write failures stay fatal unless
// they only failed because the recovery budget ended; NotFound, name and
// ownership observations keep their decision-table meaning.
func recreateRecoveryTransient(budget context.Context, err error) bool {
	if budget.Err() != nil {
		return true
	}
	var uncertain *recreateUncertain
	if errors.As(err, &uncertain) {
		return false
	}
	return errdefs.IsUnavailable(err) || client.IsErrConnectionFailed(err) || errors.Is(err, context.DeadlineExceeded)
}

// RecoverRecreates runs before generic startup settlement and admission. Only
// unresolved local receipts are recovered; a write failure leaves them excluded.
// Run IDs whose Docker observation stayed transient within the bound are
// returned as pending: the caller holds them as unknown for the next start.
func (s *ContainerService) RecoverRecreates(ctx context.Context) ([]dto.RecreateRecovery, []string, error) {
	if s.operations == nil || s.operations.db == nil {
		return nil, nil, nil
	}
	var journals []RecreateJournal
	err := s.operations.db.WithContext(ctx).Table("container_recreate_journal").Where("run_id IN (SELECT run_id FROM runtime_receipts WHERE environment_id='0' AND settled_at IS NULL)").Order("run_id").Find(&journals).Error
	if err != nil {
		return nil, nil, err
	}
	if len(journals) == 0 {
		return nil, nil, nil
	}
	timing := recreateRecoveryTiming
	budget, cancel := context.WithTimeout(ctx, timing.budget)
	defer cancel()
	budget = context.WithValue(budget, recreateCallBudgetKey{}, timing.call)

	results := make([]dto.RecreateRecovery, 0, len(journals))
	pending := journals
	var cause error
	for attempt := 1; ; attempt++ {
		var next []RecreateJournal
		c, clientErr := s.dockerService.GetClient(budget)
		if clientErr != nil {
			next, cause = pending, clientErr
		} else {
			for i := range pending {
				j := &pending[i]
				if attempt > 1 {
					// A retry starts from the journal as committed, never from a
					// phase whose write did not happen.
					if err := s.operations.db.WithContext(ctx).Table("container_recreate_journal").First(j, "run_id=?", j.RunID).Error; err != nil {
						return nil, nil, fmt.Errorf("recreate recovery: %w", err)
					}
				}
				result, err := s.recoverRecreate(budget, c, j)
				if err == nil {
					results = append(results, result)
					continue
				}
				if !recreateRecoveryTransient(budget, err) {
					return nil, nil, fmt.Errorf("recreate recovery: %w", err)
				}
				next, cause = append(next, *j), err
			}
		}
		pending = next
		if len(pending) == 0 || attempt >= timing.attempts || budget.Err() != nil {
			break
		}
		timer := time.NewTimer(timing.pause)
		select {
		case <-timer.C:
		case <-budget.Done():
			timer.Stop()
		}
	}
	runs := make([]string, 0, len(pending))
	for i := range pending {
		slog.WarnContext(ctx, "Recreate recovery deferred to the next start; Docker unavailable", "runId", pending[i].RunID, "error", cause)
		runs = append(runs, pending[i].RunID)
	}
	return results, runs, nil
}
