// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original explicit nested ownership of a hub target admission fence.

package backup

import (
	"context"
	"sync/atomic"
)

type targetReadKey struct{}
type targetReadOwner struct {
	service *Service
	id      string
	active  atomic.Bool
}

// The hub handler holds its target fence through the admission response. Local
// capture reuses that ownership: a recursive RLock deadlocks behind a queued retire.
func (s *Service) withActiveTarget(ctx context.Context, id string, run func(context.Context) error) error {
	if owner, ok := ctx.Value(targetReadKey{}).(*targetReadOwner); ok && owner.service == s && owner.id == id && owner.active.Load() {
		return run(ctx)
	}
	return s.targets.WithActive(ctx, id, func() error {
		owner := &targetReadOwner{service: s, id: id}
		owner.active.Store(true)
		defer owner.active.Store(false)
		return run(context.WithValue(ctx, targetReadKey{}, owner))
	})
}
