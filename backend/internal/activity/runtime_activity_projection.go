// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Receipt knowledge survives legacy list and progress projections.

package activity

import (
	"context"
	"errors"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

func runtimeProjectModelInternal(model *Activity, row *RuntimeReceipt) {
	status := runtimeStatusInternal(row)
	localQueue := row.EnvironmentID == "0" && row.ExecutorGeneration == row.StoreGeneration && *row.DispatchState == "pending" && (*row.ExecutorState == "pending" || *row.ExecutorState == "accepted")
	remoteQueue := row.EnvironmentID != "0" && *row.DispatchState == "accepted" && *row.ExecutorState == "accepted"
	if status == activitytypes.StatusRunning && model.Status == activitytypes.StatusQueued && (localQueue || remoteQueue) && runtimeQueuesInternal(RuntimeOperation(*row.OperationID)) {
		status = activitytypes.StatusQueued
	}
	model.Status = status
	if row.HubDisposition != nil {
		model.MessageCode = string(activitytypes.CodeRuntimeInterrupted)
		model.Error = row.HubDispositionReason
	} else if row.ExecutorResultCode != nil {
		model.MessageCode = *row.ExecutorResultCode
	}
	if isTerminalActivityStatusInternal(status) && model.EndedAt == nil {
		model.EndedAt = copyPtrInternal(row.SettledAt)
	}
	knowledge := runtimeKnowledgeInternal(row)
	model.runtimeKnowledge = &knowledge
}

func (s *ActivityService) runtimeAnnotateActivityTxInternal(tx *gorm.DB, model *Activity) error {
	if s.runtime == nil {
		return nil
	}
	var row RuntimeReceipt
	err := tx.First(&row, "run_id = ? AND environment_id = ? AND kind = 'receipt'", model.ID, model.EnvironmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	runtimeProjectModelInternal(model, &row)
	return nil
}

func (s *ActivityService) projectRuntimeListInternal(ctx context.Context, items []activitytypes.Activity) error {
	if s.runtime == nil || len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []RuntimeReceipt
		for start := 0; start < len(ids); start += activityProjectionChunk {
			var chunk []RuntimeReceipt
			if err := tx.Where("run_id IN ? AND kind = 'receipt'", ids[start:min(start+activityProjectionChunk, len(ids))]).Find(&chunk).Error; err != nil {
				return err
			}
			rows = append(rows, chunk...)
		}
		byID := make(map[string]*RuntimeReceipt, len(rows))
		for i := range rows {
			byID[rows[i].RunID] = &rows[i]
		}
		for i, item := range items {
			row := byID[item.ID]
			if row == nil || row.EnvironmentID != item.EnvironmentID {
				continue
			}
			snapshot, err := runtimeSnapshotTxInternal(tx, row)
			if err != nil {
				return err
			}
			items[i] = snapshot.Activity
		}
		return nil
	})
}

// Receipt ownership alone suppresses agent observations, including after hub
// history clearing. Unrelated legacy summaries retain their existing merge.
func (s *ActivityService) deduplicateRuntimeRemoteInternal(ctx context.Context, environment string, remote []activitytypes.Activity) ([]activitytypes.Activity, int64, error) {
	if s.runtime == nil || len(remote) == 0 {
		return remote, 0, nil
	}
	owned := make(map[string]bool, len(remote))
	for start := 0; start < len(remote); start += 100 {
		ids := make([]string, 0, min(100, len(remote)-start))
		for _, item := range remote[start:min(start+100, len(remote))] {
			ids = append(ids, item.ID)
		}
		var matches []string
		if err := s.db.WithContext(ctx).Model(&RuntimeReceipt{}).Where("environment_id = ? AND run_id IN ? AND kind = 'receipt'", environment, ids).Pluck("run_id", &matches).Error; err != nil {
			return nil, 0, err
		}
		for _, id := range matches {
			owned[id] = true
		}
	}
	out := make([]activitytypes.Activity, 0, len(remote))
	var duplicates int64
	for _, item := range remote {
		if owned[item.ID] {
			duplicates++
			continue
		}
		out = append(out, item)
	}
	return out, duplicates, nil
}
