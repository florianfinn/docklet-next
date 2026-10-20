// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"encoding/json/v2"
	"errors"
	"slices"
	"unicode/utf8"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

func runtimeDetailTxInternal(tx *gorm.DB, row *RuntimeReceipt) (*activitytypes.RuntimeExecutionDetail, error) {
	return runtimeDetailSnapshotTxInternal(tx, row, true)
}
func runtimeDetailSnapshotTxInternal(tx *gorm.DB, row *RuntimeReceipt, executorWire bool) (*activitytypes.RuntimeExecutionDetail, error) {
	var oversized int64
	if err := tx.Model(&Activity{}).Where("id = ? AND (length(CAST(runtime_output AS BLOB)) > ? OR length(CAST(metadata AS BLOB)) > ? OR length(CAST(runtime_step_trace AS BLOB)) > 65536 OR length(CAST(latest_message AS BLOB)) > ? OR length(CAST(step AS BLOB)) > ?)", row.RunID, activitytypes.RuntimeMaxOutputBytes, 65536, 256<<10, 256<<10).Count(&oversized).Error; err != nil {
		return nil, err
	}
	if oversized != 0 {
		return nil, activitytypes.ErrRuntimeWire
	}
	snapshot, err := runtimeSnapshotTxInternal(tx, row)
	if err != nil {
		return nil, err
	}
	snapshot.LatestMessage = runtimeObservationTextInternal(snapshot.LatestMessage, 16<<10)
	snapshot.Step = runtimeObservationTextInternal(snapshot.Step, 4096)
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
	if err := tx.Where("activity_id = ? AND length(CAST(message AS BLOB)) <= 65536 AND (payload IS NULL OR length(CAST(payload AS BLOB)) <= 65536) AND length(CAST(step AS BLOB)) <= 4096", row.RunID).Order("created_at DESC, id DESC").Limit(100).Find(&messages).Error; err != nil {
		return nil, err
	}
	messageBytes := 0
	for _, message := range messages {
		dto := activityMessageToDTOInternal(&message)
		encoded, err := json.Marshal(dto)
		if err != nil {
			return nil, err
		}
		if messageBytes+len(encoded) > 256<<10 {
			break
		}
		messageBytes += len(encoded)
		detail.Messages = append(detail.Messages, dto)
	}
	slices.Reverse(detail.Messages)
	var steps []ActivityMessage
	if err := tx.Raw(`SELECT id, step, code, created_at FROM (
 SELECT id, step, code, created_at, LAG(step) OVER (ORDER BY created_at, id) AS previous_step
 FROM activity_messages WHERE activity_id = ? AND step <> '' AND length(CAST(step AS BLOB)) <= 4096) AS transitions
 WHERE previous_step IS DISTINCT FROM step ORDER BY created_at DESC, id DESC LIMIT ?`, row.RunID, maxStepTransitions).Scan(&steps).Error; err != nil {
		return nil, err
	}
	slices.Reverse(steps)
	detail.StepTrace = append([]activitytypes.StepTraceEntry{}, stepTraceInternal(steps, &model)...)
	if row.EnvironmentID != "0" {
		detail.StepTrace = append([]activitytypes.StepTraceEntry{}, model.RuntimeStepTrace...)
	}
	for len(detail.StepTrace) > 0 {
		encoded, err := json.Marshal(detail.StepTrace)
		if err != nil {
			return nil, err
		}
		if len(encoded) <= 65536 {
			break
		}
		detail.StepTrace = detail.StepTrace[1:]
	}
	for {
		encoded, err := json.Marshal(detail)
		if err != nil {
			return nil, err
		}
		if len(encoded) <= activitytypes.RuntimeMaxBodyBytes-1024 {
			break
		}
		if len(detail.Messages) > 0 {
			detail.Messages = detail.Messages[1:]
			continue
		}
		if len(detail.StepTrace) > 0 {
			detail.StepTrace = detail.StepTrace[1:]
			continue
		}
		return nil, activitytypes.ErrRuntimeWire
	}
	validation := *detail
	if !executorWire {
		// The hub owns host routing and unknown knowledge; observation/window
		// bounds remain identical to the authenticated executor carrier.
		validation.Activity.EnvironmentID = "0"
		validation.Activity.OutcomeState = activitytypes.RuntimeOutcomeKnown
	}
	if err := activitytypes.ValidateRuntimeExecutionDetail(&validation, row.RunID); err != nil {
		return nil, err
	}
	return detail, nil
}

func runtimeObservationTextInternal(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	const marker = " [truncated]"
	end := limit - len(marker)
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + marker
}
