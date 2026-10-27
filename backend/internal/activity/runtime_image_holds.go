// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Advisory hub point holds and bounded agent-response validation for #229.

package activity

import (
	"context"
	"errors"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

// ReadPointHolds is an advisory DB read. It takes no runtime mutex and cannot
// grant executor authority. Old registrations retain holds on the same UUID.
func (s *ActivityService) ReadPointHolds(ctx context.Context, env string, ids []string) (map[string]dto.RestorePointHoldState, error) {
	out := map[string]dto.RestorePointHoldState{}
	if len(ids) == 0 {
		return out, nil
	}
	if s == nil || s.db == nil {
		return nil, imageDTO.Failure("update.persistence-failed", 500)
	}
	var rows []struct {
		PointID       string
		ExecutorState string
	}
	err := s.db.WithContext(ctx).Table("runtime_image_intents AS i").Select("json_extract(i.payload, '$.request.pointId') AS point_id, r.executor_state").Joins("JOIN runtime_receipts r ON r.run_id = i.run_id").Where("r.kind = 'receipt' AND r.environment_id = ? AND r.settled_at IS NULL AND json_extract(i.payload, '$.request.pointId') IN ?", env, ids).Scan(&rows).Error
	if err != nil {
		return nil, imageDTO.Failure("update.persistence-failed", 500)
	}
	for _, row := range rows {
		state := dto.HoldActive
		switch row.ExecutorState {
		case "unknown":
			state = dto.HoldUnknown
		case "pending", "accepted", "executing":
		default:
			return nil, imageDTO.Neutral(errors.New("invalid point receipt"), "update.persistence-failed", 500)
		}
		out[row.PointID] = dto.MergePointHold(out[row.PointID], state)
	}
	return out, nil
}
