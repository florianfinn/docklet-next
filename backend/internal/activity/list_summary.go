// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

package activity

import (
	"context"
	"encoding/json/v2"
	"slices"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

const activityProjectionChunk = 100

// Project only authorized list identities. Retained output and messages never
// enter this projection; the remote receipt owns its executor trace window.
func (s *ActivityService) listSummariesInternal(ctx context.Context, items []activitytypes.Activity, include bool, remote map[string]activitytypes.ActivitySummary) ([]activitytypes.ActivitySummary, error) {
	out := make([]activitytypes.ActivitySummary, len(items))
	for i, item := range items {
		out[i].Activity = item
	}
	if !include || len(items) == 0 {
		return out, nil
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(items); start += activityProjectionChunk {
			end := min(start+activityProjectionChunk, len(items))
			ids := make([]string, 0, end-start)
			for _, item := range items[start:end] {
				ids = append(ids, item.ID)
			}
			var receipts []RuntimeReceipt
			if s.runtime != nil {
				if err := tx.Where("run_id IN ? AND kind = 'receipt'", ids).Find(&receipts).Error; err != nil {
					return err
				}
			}
			owned := make(map[string]RuntimeReceipt)
			for _, row := range receipts {
				owned[row.RunID] = row
			}
			ordinary, localRuntime, remoteRuntime := []string{}, []string{}, []string{}
			for i := start; i < end; i++ {
				item := items[i]
				row, receipt := owned[item.ID]
				if receipt && row.EnvironmentID == item.EnvironmentID {
					out[i].Recovery = &activitytypes.ActivitySummaryRecovery{Disposition: row.HubDisposition, UnknownReason: row.UnknownReason}
					if row.EnvironmentID == "0" {
						localRuntime = append(localRuntime, item.ID)
					} else {
						remoteRuntime = append(remoteRuntime, item.ID)
					}
				} else if summary, ok := remote[item.ID]; ok {
					out[i].StepTrace = summary.StepTrace
					// Recovery belongs to hub receipts, never to an unrelated agent claim.
				} else {
					ordinary = append(ordinary, item.ID)
				}
			}
			byID := make(map[string][]ActivityMessage)
			for _, group := range []struct {
				ids     []string
				bounded bool
			}{{ordinary, false}, {localRuntime, true}} {
				if len(group.ids) == 0 {
					continue
				}
				predicate := ""
				if group.bounded {
					predicate = " AND length(CAST(step AS BLOB)) <= 4096"
				}
				var messages []ActivityMessage
				query := `SELECT id, activity_id, step, code, created_at FROM (
     SELECT *, ROW_NUMBER() OVER (PARTITION BY activity_id ORDER BY created_at DESC, id DESC) AS rank FROM (
      SELECT * FROM (
       SELECT id, activity_id, step, code, created_at, LAG(step) OVER (PARTITION BY activity_id ORDER BY created_at, id) AS previous_step
       FROM activity_messages WHERE activity_id IN ? AND step <> ''` + predicate + `
      ) WHERE previous_step IS DISTINCT FROM step
     )
    ) WHERE rank <= ? ORDER BY activity_id, created_at, id`
				if err := tx.Raw(query, group.ids, maxStepTransitions).Scan(&messages).Error; err != nil {
					return err
				}
				for _, msg := range messages {
					byID[msg.ActivityID] = append(byID[msg.ActivityID], msg)
				}
			}
			retained := make(map[string]RuntimeStepTrace)
			modelsByID := make(map[string]Activity)
			if len(remoteRuntime)+len(localRuntime) > 0 {
				var models []Activity
				if err := tx.Select("id", "status", "ended_at", "runtime_step_trace").Where("id IN ?", append(remoteRuntime, localRuntime...)).Find(&models).Error; err != nil {
					return err
				}
				for _, model := range models {
					retained[model.ID] = model.RuntimeStepTrace
					modelsByID[model.ID] = model
				}
			}
			for i := start; i < end; i++ {
				item := items[i]
				if _, ok := remote[item.ID]; ok && out[i].Recovery == nil {
					continue
				}
				model := Activity{Status: item.Status, EndedAt: item.EndedAt}
				if stored, ok := modelsByID[item.ID]; ok {
					model = stored
				}
				trace := stepTraceInternal(byID[item.ID], &model)
				if out[i].Recovery != nil {
					if item.EnvironmentID != "0" {
						trace = append([]activitytypes.StepTraceEntry{}, retained[item.ID]...)
					}
					var err error
					trace, err = boundedSummaryTraceInternal(trace)
					if err != nil {
						return err
					}
				}
				out[i].StepTrace = trace
			}
		}
		return nil
	})
	return out, err
}

func boundedSummaryTraceInternal(trace []activitytypes.StepTraceEntry) ([]activitytypes.StepTraceEntry, error) {
	if len(trace) > maxStepTransitions {
		trace = trace[len(trace)-maxStepTransitions:]
	}
	trace = slices.Clone(trace)
	if trace == nil {
		trace = []activitytypes.StepTraceEntry{}
	}
	for len(trace) > 0 {
		encoded, err := json.Marshal(trace)
		if err != nil {
			return nil, err
		}
		if len(encoded) <= 65536 {
			break
		}
		trace = trace[1:]
	}
	return trace, nil
}
