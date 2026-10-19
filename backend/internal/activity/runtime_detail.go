// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"errors"
	"slices"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

func runtimeDetailTxInternal(tx *gorm.DB, row *RuntimeReceipt) (*activitytypes.RuntimeExecutionDetail, error) {
	snapshot, err := runtimeSnapshotTxInternal(tx, row)
	if err != nil {
		return nil, err
	}
	detail := &activitytypes.RuntimeExecutionDetail{Activity: snapshot, Messages: []activitytypes.Message{}, StepTrace: []activitytypes.StepTraceEntry{}, Output: []activitytypes.RuntimeExecutionOutputLine{}, RetainedStartSeq: row.OutputNextSeq}
	if row.OutputNextSeq == 0 {
		return nil, errRuntimeInvalidInternal
	}
	var model Activity
	err = tx.First(&model, "id = ? AND environment_id = ?", row.RunID, row.EnvironmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		detail.OutputTruncated = true
		return detail, nil
	}
	if err != nil {
		return nil, err
	}
	detail.Output = append(detail.Output, model.RuntimeOutput...)
	detail.OutputTruncated = model.OutputTruncated
	if len(detail.Output) > 0 {
		detail.RetainedStartSeq = detail.Output[0].OutputSeq
	}
	var messages []ActivityMessage
	if err := tx.Where("activity_id = ?", row.RunID).Order("created_at DESC, id DESC").Limit(100).Find(&messages).Error; err != nil {
		return nil, err
	}
	slices.Reverse(messages)
	for _, message := range messages {
		detail.Messages = append(detail.Messages, activityMessageToDTOInternal(&message))
	}
	var steps []ActivityMessage
	if err := tx.Raw(`SELECT id, step, code, created_at FROM (
 SELECT id, step, code, created_at, LAG(step) OVER (ORDER BY created_at, id) AS previous_step
 FROM activity_messages WHERE activity_id = ? AND step <> '') AS transitions
 WHERE previous_step IS DISTINCT FROM step ORDER BY created_at DESC, id DESC LIMIT ?`, row.RunID, maxStepTransitions).Scan(&steps).Error; err != nil {
		return nil, err
	}
	slices.Reverse(steps)
	detail.StepTrace = stepTraceInternal(steps, &model)
	if err := activitytypes.ValidateRuntimeExecutionDetail(detail, row.RunID); err != nil {
		return nil, err
	}
	return detail, nil
}
