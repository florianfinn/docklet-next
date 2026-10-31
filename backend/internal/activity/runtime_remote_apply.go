// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	policies "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Remote evidence and its bounded replacement projection share one COMMIT. A
// manual disposition is immutable; later executor evidence only updates receipt
// knowledge and remains separately visible through the recovery detail helper.
func (s *RuntimeService) applyRemoteInternal(ctx context.Context, ref RuntimeRunReference, outcome activitytypes.RuntimeExecutionOutcome, fromQuery bool) (activitytypes.RuntimeSnapshot, error) {
	if activitytypes.ValidateRuntimeOutcome(outcome, outcome.Identity) != nil || outcome.State == activitytypes.RuntimeAnswerNotReceived {
		return activitytypes.RuntimeSnapshot{}, activitytypes.ErrRuntimeWire
	}
	outcome = checkedRemoteLifecycle(outcome)
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	lock := s.activity.publishLockInternal(ref.RunID)
	lock.Lock()
	defer lock.Unlock()
	var snapshot activitytypes.RuntimeSnapshot
	var final *ActivityMessage
	var changed bool
	err := s.ownerTransactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		id := runtimeRowIdentityInternal(row)
		if runtimeWireIdentityInternal(row) != outcome.Identity || *row.DispatchState == "pending" {
			return &runtimeConflictInternal{}
		}
		d := outcome.Detail
		kind, ok := runtimeActivityTypeInternal(*row.OperationID, *row.TargetKind)
		if !ok || d.Activity.Type != kind || d.Activity.ResourceID == nil || *d.Activity.ResourceID != *row.TargetID || d.Activity.ResourceType == nil || *d.Activity.ResourceType != *row.TargetKind || d.Activity.StartedBy == nil || d.Activity.StartedBy.UserID != *row.CallerID {
			return &runtimeConflictInternal{}
		}
		terminal := outcome.State == activitytypes.RuntimeAnswerTerminal
		if row.ExecutorSettledAt != nil || (!fromQuery && row.LastReconciledAt != nil && !terminal) {
			snapshot, err = runtimeSnapshotTxInternal(tx, row)
			return err
		}
		// Active is authoritative receipt acceptance, not proof that effects
		// started. The carrier does not supply a separate executing timestamp.
		state := "accepted"
		if *row.ExecutorState == "executing" {
			state = "executing"
		}
		evidence := runtimeEvidenceInternal{State: state, StartedAt: copyPtrInternal(row.ExecutorStartedAt)}
		if terminal {
			evidence.State = string(d.Activity.Status)
			evidence.SettledAt = d.Activity.EndedAt
			evidence.ResultCode = new(d.Activity.MessageCode)
			evidence.Error = copyPtrInternal(d.Activity.Error)
			switch evidence.State {
			case "success":
				if d.Activity.MessageCode != string(activitytypes.CodeActivitySuccess) {
					return activitytypes.ErrRuntimeWire
				}
			case "failed":
				if d.Activity.MessageCode != string(activitytypes.CodeRuntimeFailed) && d.Activity.MessageCode != string(activitytypes.CodeRuntimeInterrupted) && d.Activity.MessageCode != "container.recreate-recovered" && d.Activity.MessageCode != "container.recreate-ambiguous" {
					return activitytypes.ErrRuntimeWire
				}
			case "cancelled":
				if d.Activity.MessageCode != string(activitytypes.CodeRuntimeCancelled) {
					return activitytypes.ErrRuntimeWire
				}
			default:
				return activitytypes.ErrRuntimeWire
			}
		}
		row, err = runtimeEvidenceTxInternal(tx, now, id, evidence)
		if err != nil {
			return err
		}
		if *row.ExecutorState != evidence.State || !runtimeEqualInternal(row.ExecutorSettledAt, runtimeExecutorTimeInternal(evidence.SettledAt)) || !runtimeEqualInternal(row.ExecutorResultCode, evidence.ResultCode) {
			return &runtimeConflictInternal{}
		}
		if terminal {
			if err := runtimeRecreateResultTx(tx, row, d.Activity.Result); err != nil {
				return err
			}
		}
		if fromQuery {
			if err := runtimeUpdateTxInternal(tx, now, row, map[string]any{"last_reconciled_at": now}); err != nil {
				return err
			}
		}
		if *row.DispatchState == "dispatching" {
			if err := runtimeUpdateTxInternal(tx, now, row, map[string]any{"dispatch_state": "accepted"}); err != nil {
				return err
			}
		}
		if row.HubDisposition != nil {
			snapshot, err = runtimeSnapshotTxInternal(tx, row)
			return err
		}
		var model Activity
		if err := runtimeEnsureActivityTx(tx, row, &model); err != nil {
			return err
		}
		// Terminal always wins against an outstanding active acknowledgment. Within
		// an active state the durable output cursor cannot be replaced by an older one.
		next := d.RetainedStartSeq
		if len(d.Output) > 0 {
			next = d.Output[len(d.Output)-1].OutputSeq + 1
		}
		if next < row.OutputNextSeq && !terminal {
			snapshot, err = runtimeSnapshotTxInternal(tx, row)
			return err
		}
		legacy := make(OutputLines, 0, len(d.Output))
		for _, line := range d.Output {
			legacy = append(legacy, line.OutputLine)
		}
		updates := map[string]any{"status": d.Activity.Status, "progress": d.Activity.Progress, "step": d.Activity.Step, "latest_message": d.Activity.LatestMessage, "message_code": d.Activity.MessageCode, "affected_services": servicesJSONInternal(d.Activity.AffectedServices), "metadata": database.JSON(d.Activity.Metadata), "runtime_step_trace": RuntimeStepTrace(d.StepTrace), "output": legacy, "runtime_output": RuntimeOutputLines(d.Output), "output_truncated": model.OutputTruncated || d.OutputTruncated, "updated_at": now, "error": d.Activity.Error}
		if terminal {
			if err := runtimeNotifyFailureTx(tx, now, row, d.Activity.MessageCode, false, false); err != nil {
				return err
			}
			updates["ended_at"], updates["duration_ms"] = now, now.Sub(model.StartedAt).Milliseconds()
			level := activitytypes.MessageLevelSuccess
			if d.Activity.Status == activitytypes.StatusFailed {
				level = activitytypes.MessageLevelError
			}
			if d.Activity.Status == activitytypes.StatusCancelled {
				level = activitytypes.MessageLevelWarning
			}
			final = &ActivityMessage{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: new(now)}, ActivityID: row.RunID, Code: d.Activity.MessageCode, Level: level, Message: "Remote workflow settled; effects unverified", Payload: database.JSON{"actorId": *row.CallerID, "operation": *row.OperationID, "effectVerified": false}}
			if err := runtimeReceiverCreateTxInternal(tx, final); err != nil {
				return err
			}
		}
		result := tx.Model(&Activity{}).Where("id = ? AND status IN ?", row.RunID, []activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning}).UpdateColumns(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return &runtimeConflictInternal{}
		}
		// Replace only this bounded remote window, preserving hub audit/final rows.
		if err := tx.Where("activity_id = ? AND runtime_remote = 1", row.RunID).Delete(&ActivityMessage{}).Error; err != nil {
			return err
		}
		for _, m := range d.Messages {
			if m.Code == string(activitytypes.CodeRunStarted) || (terminal && m.Code == d.Activity.MessageCode) {
				continue
			}
			remote := &ActivityMessage{BaseModel: database.BaseModel{ID: m.ID, CreatedAt: m.CreatedAt, UpdatedAt: new(now)}, ActivityID: row.RunID, Level: m.Level, Code: m.Code, Message: m.Message, Step: m.Step, Payload: database.JSON(m.Payload), RuntimeRemote: true}
			if err := runtimeReceiverCreateTxInternal(tx, remote); err != nil {
				return err
			}
		}
		if err := runtimeUpdateTxInternal(tx, now, row, map[string]any{"output_next_seq": max(row.OutputNextSeq, next)}); err != nil {
			return err
		}
		if row.OutputNextSeq < next {
			return &runtimeConflictInternal{}
		}
		snapshot, err = runtimeSnapshotTxInternal(tx, row)
		changed = err == nil
		return err
	})
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if changed {
		if final != nil {
			s.activity.publishMessageInternal(snapshot.EnvironmentID, snapshot.Type, activityMessageToDTOInternal(final))
		}
		s.activity.publishActivityInternal(snapshot.Activity)
	}
	if snapshot.Result != nil && s.activity.runtimeHTTP != nil && s.activity.runtimeHTTP.ports.InvalidateInventory != nil {
		s.activity.runtimeHTTP.ports.InvalidateInventory(ref.Store.EnvironmentID)
	}
	return snapshot, nil
}
func runtimeWireIdentityInternal(row *RuntimeReceipt) activitytypes.RuntimeRunIdentity {
	return activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: row.ManagerID, RegistrationID: row.RegistrationID, ExecutorGeneration: row.ExecutorGeneration}, RunID: row.RunID, RequestDigest: row.RequestDigest}
}

// Hub dispositions write one final Activity/audit message. They never manufacture
// executor completion; the independent receipt evidence remains authoritative.
func runtimeHubDispositionTxInternal(tx *gorm.DB, now time.Time, row *RuntimeReceipt, disposition runtimeDispositionInternal) (activitytypes.RuntimeSnapshot, *ActivityMessage, error) {
	empty := activitytypes.RuntimeSnapshot{}
	if row.HubDisposition != nil {
		snapshot, err := runtimeSnapshotTxInternal(tx, row)
		return snapshot, nil, err
	}
	if row.ExecutorSettledAt != nil {
		return empty, nil, &runtimeConflictInternal{}
	}
	row, err := runtimeDispositionTxInternal(tx, now, runtimeRowIdentityInternal(row), disposition)
	if err != nil {
		return empty, nil, err
	}
	if row.HubDisposition == nil || *row.HubDisposition != disposition.Disposition || row.SettledAt == nil {
		return empty, nil, &runtimeConflictInternal{}
	}
	code := activitytypes.CodeRuntimeInterrupted
	status := activitytypes.StatusFailed
	if disposition.Disposition == "cancelled" {
		code = activitytypes.CodeRuntimeCancelled
		status = activitytypes.StatusCancelled
	}
	result := tx.Model(&Activity{}).Where("id = ? AND environment_id = ? AND status IN ?", row.RunID, row.EnvironmentID, []activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning}).UpdateColumns(map[string]any{"status": status, "ended_at": now, "latest_message": disposition.Reason, "error": disposition.Reason, "message_code": string(code), "updated_at": now})
	if result.Error != nil {
		return empty, nil, result.Error
	}
	if result.RowsAffected != 1 {
		return empty, nil, &runtimeConflictInternal{}
	}
	message := &ActivityMessage{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: new(now)}, ActivityID: row.RunID, Level: activitytypes.MessageLevelWarning, Code: string(code), Message: disposition.Reason, Payload: database.JSON{"actorId": disposition.By, "disposition": disposition.Disposition, "releaseAcknowledged": disposition.ReleaseAcknowledged, "effectVerified": false}}
	if err := runtimeReceiverCreateTxInternal(tx, message); err != nil {
		return empty, nil, err
	}
	if disposition.Disposition == "released" && disposition.ReleaseAcknowledged {
		if err := tx.Model(&policies.Child{}).Where("runtime_id = ? AND status = ?", row.RunID, "admitted").Update("status", "settled").Error; err != nil {
			return empty, nil, err
		}
	}
	if err := runtimeNotifyFailureTx(tx, now, row, string(code), false, true); err != nil {
		return empty, nil, err
	}
	snapshot, err := runtimeSnapshotTxInternal(tx, row)
	return snapshot, message, err
}
func (s *RuntimeService) remoteDispositionInternal(ctx context.Context, ref RuntimeRunReference, disposition runtimeDispositionInternal) (activitytypes.RuntimeSnapshot, error) {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	lock := s.activity.publishLockInternal(ref.RunID)
	lock.Lock()
	defer lock.Unlock()
	var snapshot activitytypes.RuntimeSnapshot
	var message *ActivityMessage
	err := s.ownerTransactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		snapshot, message, err = runtimeHubDispositionTxInternal(tx, now, row, disposition)
		return err
	})
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if message != nil {
		s.activity.publishMessageInternal(snapshot.EnvironmentID, snapshot.Type, activityMessageToDTOInternal(message))
		s.activity.publishActivityInternal(snapshot.Activity)
	}
	return snapshot, nil
}
