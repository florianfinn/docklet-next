// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Activation closes all local continuity decisions before readiness.

package activity

import "context"

// InitializeLocalStartup runs only after the prior process/handlers ended.
// Matching abandoned workflows settle interrupted; unmatched local ownership
// remains unknown and held for explicit administrator recovery, never execution.
func (s *RuntimeService) InitializeLocalStartup(ctx context.Context, binding RuntimeStoreBinding) error {
	if s.localExecution != nil {
		if err := s.recoverRecreatesStartup(ctx); err != nil {
			return err
		}
	}
	if _, err := s.SettleLocalStartup(ctx, binding); err != nil {
		return err
	}
	s.activity.runningMu.Lock()
	if len(s.owners) != 0 {
		s.activity.runningMu.Unlock()
		return ErrRuntimeStartupOwnersLive
	}
	var rows []RuntimeReceipt
	err := s.activity.db.WithContext(ctx).Where("kind = 'receipt' AND environment_id = '0' AND settled_at IS NULL").Order("run_id").Find(&rows).Error
	s.activity.runningMu.Unlock()
	if err != nil {
		return &RuntimeStoreError{Cause: err}
	}
	for i := range rows {
		row := &rows[i]
		if _, err := s.remoteUnknownInternal(ctx, RuntimeRunReference{Store: runtimeStoreFromRowInternal(row), RunID: row.RunID}, "executor-continuity"); err != nil {
			return err
		}
	}
	return nil
}
