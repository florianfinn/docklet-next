// Source: docklet-next/issues/441
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original hub failed-candidate settlement integration.

package updater

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	a "github.com/florianfinn/docklet-next/types/activity"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
	"strings"
)

// Every waiter and reconciler shares atomic settlement. Reload after admission:
// PrepareChild returns a prepared child without its committed runtime identity.
func (s *UpdaterService) settleImagePolicySnapshot(ctx context.Context, p *imagePolicyService, target string, child policy.Child, snapshot a.RuntimeSnapshot) (st.TargetOutcome, bool) {
	id := child.RuntimeID
	if id == "" {
		id = child.ID
	}
	out, unknown := imagePolicySnapshotTarget(target, id, snapshot)
	if unknown {
		return out, true
	}
	err := p.store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current policy.Child
		if err := tx.First(&current, "id = ?", child.ID).Error; err != nil {
			return err
		}
		if current.Status == "settled" {
			return nil
		}
		if current.RuntimeID != id {
			return errors.New("child runtime identity changed")
		}
		checked := policy.CheckedChildResult{Code: out.Code, SettledAt: *policyNow()}
		if snapshot.EndedAt != nil {
			checked.SettledAt = *snapshot.EndedAt
		}
		if result := snapshot.ImageLifecycle; result != nil {
			checked.Recovery, checked.Applied = result.Recovery, result.Applied
			if result.Recovery != nil && result.Recovery.Outcome == "restored" {
				var row activity.RuntimeReceipt
				if err := tx.First(&row, "run_id = ?", id).Error; err != nil {
					return err
				}
				if row.TargetKind == nil {
					return errors.New("missing admitted target kind")
				}
				if *row.TargetKind == "container" {
					if row.TargetName == nil {
						return errors.New("missing admitted container name")
					}
					checked.TargetIdentity = strings.TrimPrefix(*row.TargetName, "/")
				} else {
					var retained activity.RuntimeImageIntent
					if err := tx.First(&retained, "run_id = ?", id).Error; err != nil {
						return err
					}
					var intent dto.LifecycleIntent
					if err := json.Unmarshal([]byte(retained.Payload), &intent); err != nil {
						return err
					}
					checked.TargetIdentity = intent.TargetIdentity
				}
			}
		}
		return policy.SettleChildTx(tx, current, checked)
	})
	if err != nil {
		return policyTargetWithActivity(target, id, st.Failed, "update.persistence-failed"), true
	}
	return out, false
}
