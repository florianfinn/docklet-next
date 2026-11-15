// Source: docklet-next/issues/372
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original owned write path for host status transitions.

package environment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"
)

// hostStatusTransition names every kind of status write on the hosts table. Migration 004
// enforces the verdict rule in the database; this file is the single place that writes
// status and decides which transitions count as a check of the current registration.
type hostStatusTransition string

const (
	statusCompatibilityResult hostStatusTransition = "compatibility-result"
	statusCredentialRejected  hostStatusTransition = "credential-rejected"
	statusRegistrationLinked  hostStatusTransition = "registration-linked"
	statusTransportConnected  hostStatusTransition = "transport-connected"
	statusTransportLost       hostStatusTransition = "transport-lost"
	statusProbe               hostStatusTransition = "probe"
	statusStartupReconcile    hostStatusTransition = "startup-reconcile"
	statusLocalHeartbeat      hostStatusTransition = "local-heartbeat"
)

type hostStatusOutcome string

const (
	hostStatusApplied  hostStatusOutcome = "applied"
	hostStatusNoop     hostStatusOutcome = "noop"
	hostStatusRejected hostStatusOutcome = "rejected"
)

// ErrHostStatusVerdict reports a write the database verdict rule refused.
var ErrHostStatusVerdict = errors.New("host.status-verdict")

// hostStatusWrite is one status write. ID is empty for multi-row writes, which then
// leave per-host caches to their callers.
type hostStatusWrite struct {
	Transition hostStatusTransition
	ID         string
	Scope      func(*gorm.DB) *gorm.DB
	Updates    map[string]any
}

// checksRegistration is true for results of the compatibility check of the current
// registration, the only transitions that may replace the update-required verdict
// without a credential change.
func (t hostStatusTransition) checksRegistration() bool {
	return t == statusCompatibilityResult || t == statusCredentialRejected
}

func (s *EnvironmentService) writeHostStatusInternal(ctx context.Context, write hostStatusWrite) (hostStatusOutcome, error) {
	updates := write.Updates
	if write.Transition.checksRegistration() {
		updates["status_check_revision"] = gorm.Expr("status_check_revision + 1")
	}
	query := s.db.WithContext(ctx).Model(&Environment{})
	if write.ID != "" {
		query = query.Where("id = ?", write.ID)
	}
	if write.Scope != nil {
		query = write.Scope(query)
	}
	result := query.Updates(updates)
	outcome := hostStatusApplied
	var err error
	switch {
	case result.Error != nil && strings.Contains(result.Error.Error(), string(ErrHostStatusVerdict.Error())):
		outcome = hostStatusRejected
		err = fmt.Errorf("%s write: %w", write.Transition, ErrHostStatusVerdict)
		slog.WarnContext(ctx, "Host status write refused by the verdict rule", "environmentId", write.ID, "transition", write.Transition)
	case result.Error != nil:
		return hostStatusRejected, result.Error
	case result.RowsAffected == 0:
		outcome = hostStatusNoop
	}
	if s.statusWriteObserver != nil {
		s.statusWriteObserver(write.Transition, outcome)
	}
	if outcome != hostStatusApplied || write.ID == "" {
		return outcome, err
	}
	s.invalidateEnvironmentCacheInternal(write.ID)
	if status, ok := updates["status"].(string); ok {
		s.remoteEnvs.update(write.ID, func(env *Environment) { env.Status = status })
	}
	return outcome, nil
}
