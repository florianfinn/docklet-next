// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: One-transaction hub detail and immutable manual recovery evidence.

package activity

import (
	"context"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
	"time"
)

func (s *RuntimeService) GetRuntimeHubDetail(ctx context.Context, ref RuntimeRunReference) (*activitytypes.RuntimeHubDetail, error) {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	var out *activitytypes.RuntimeHubDetail
	err := s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		detail, err := runtimeDetailSnapshotTxInternal(tx, row, false)
		if err != nil {
			return err
		}
		out = &activitytypes.RuntimeHubDetail{RuntimeExecutionDetail: *detail, Recovery: activitytypes.RuntimeRecoveryEvidence{
			UnknownReason: row.UnknownReason, ExecutorState: *row.ExecutorState, ExecutorResultCode: row.ExecutorResultCode, ExecutorError: row.ExecutorError, ExecutorSettledAt: row.ExecutorSettledAt,
			Disposition: row.HubDisposition, DispositionBy: row.HubDispositionBy, DispositionReason: row.HubDispositionReason, DispositionAt: row.HubDispositionAt, ReleaseAcknowledged: row.ReleaseAcknowledged}}
		return nil
	})
	return out, err
}
