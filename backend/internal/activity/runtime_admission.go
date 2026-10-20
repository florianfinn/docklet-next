// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant durable-first admission, parent arbitration and atomic coded evidence.

package activity

import (
	"context"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Admit requires trusted canonical identity plus current operation/read grants.
// A started result confirms durable acceptance, not completed execution.
func (s *RuntimeService) Admit(ctx context.Context, command RuntimeCommand) (RuntimeAdmissionResult, error) {
	if err := s.checkInternal(); err != nil {
		return RuntimeAdmissionResult{}, err
	}
	canonical, err := runtimeCanonicalizeInternal(command)
	if err != nil {
		return RuntimeAdmissionResult{}, err
	}
	// Known keys and held-scope answers are durable, including while identity I/O
	// is unavailable. New effects alone require a fresh read outside runningMu/TX.
	result, found, err := s.admissionPassInternal(ctx, canonical, false)
	if err != nil || found {
		return result, err
	}
	if s.localExecution != nil && canonical.command.Store.EnvironmentID == "0" && canonical.command.Store != s.localExecution.Store {
		return RuntimeAdmissionResult{}, &runtimeConflictInternal{}
	}
	if s.resolver == nil {
		return RuntimeAdmissionResult{}, &RuntimeTargetError{Code: "runtime.target-unavailable"}
	}
	target := canonical.command.Target
	// Give the resolver a separate copy of nullable fields as well.
	target.ParentProjectID, target.Name = copyPtrInternal(target.ParentProjectID), copyPtrInternal(target.Name)
	resolved, err := s.resolver.ResolveRuntimeTarget(context.WithValue(ctx, runtimeEnvironmentContextKey{}, command.Store.EnvironmentID), target)
	if err != nil {
		return RuntimeAdmissionResult{}, &RuntimeTargetError{Code: "runtime.target-unavailable", Cause: err}
	}
	if err := ctx.Err(); err != nil {
		return RuntimeAdmissionResult{}, err
	}
	if resolved.ComposeManaged && resolved.Target.ParentProjectID == nil {
		return RuntimeAdmissionResult{}, &RuntimeTargetError{Code: string(activitytypes.CodeRuntimeParentUnresolved)}
	}
	if !runtimeTargetValidInternal(resolved.Target) || resolved.Target.Kind != target.Kind || resolved.Target.ID != target.ID ||
		!runtimeEqualInternal(resolved.Target.ParentProjectID, canonical.command.Target.ParentProjectID) {
		return RuntimeAdmissionResult{}, &RuntimeTargetError{Code: "runtime.target-changed"}
	}
	canonical.command.Target.Name = copyPtrInternal(resolved.Target.Name)
	result, _, err = s.admissionPassInternal(ctx, canonical, true)
	return result, err
}

// Commit and local registration share runningMu. Retries/joins never register,
// and post-commit owner failures cannot turn durable acceptance into refusal.
func (s *RuntimeService) admissionPassInternal(ctx context.Context, c runtimeCanonicalCommandInternal, fresh bool) (RuntimeAdmissionResult, bool, error) {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	if s.intakePaused {
		return RuntimeAdmissionResult{}, false, ErrRuntimeNotReady
	}
	var result RuntimeAdmissionResult
	var found bool
	err := s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		if err := runtimeTargetPresentTxInternal(tx, c.command.Store); err != nil {
			return err
		}
		row, err := runtimeLookupBindingTxInternal(tx, c.identity(""), c.binding())
		code := activitytypes.CodeRunJoined
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row, err = runtimeActiveConflictTxInternal(tx, c)
			if err != nil {
				return err
			}
			if row != nil {
				// Identity continuity is required even for an authorized different-key join.
				id := c.identity(row.RunID)
				if !runtimeMatchesInternal(row, id) {
					return &runtimeConflictInternal{}
				}
				row, err = runtimeJoinTxInternal(tx, now, id, c.binding())
				if err != nil {
					return err
				}
				if err := runtimeAdmissionMessageTxInternal(tx, now, row, c, code); err != nil {
					return err
				}
			} else if !fresh {
				return nil
			} else {
				runID := c.command.ProposedRunID
				if runID == "" {
					runID = uuid.NewString()
				}
				target := c.command.Target
				row, err = runtimeAdmitTxInternal(tx, now, runtimeAdmissionInternal{Identity: c.identity(runID), Binding: c.binding(),
					OperationID: string(c.command.Operation), TargetKind: target.Kind, TargetID: target.ID,
					TargetName: target.Name, ParentProjectID: target.ParentProjectID, GateScope: c.gateScope, GateKey: c.gateKey})
				if err != nil {
					return err
				}
				if s.localExecution != nil && c.command.Store == s.localExecution.Store && runtimeQueuesInternal(c.command.Operation) {
					changed := tx.Model(&Activity{}).Where("id = ? AND environment_id = ?", row.RunID, row.EnvironmentID).
						UpdateColumns(map[string]any{"status": activitytypes.StatusQueued, "updated_at": now})
					if changed.Error != nil {
						return changed.Error
					}
					if changed.RowsAffected != 1 {
						return &runtimeConflictInternal{}
					}
				}
				if c.command.Actor.DisplayName != nil {
					// Keep the transaction's clock sample; the generic model's update
					// hook otherwise replaces UpdatedAt with the wall clock.
					if err := tx.Model(&Activity{}).Where("id = ?", row.RunID).UpdateColumns(map[string]any{
						"started_by_display_name": *c.command.Actor.DisplayName, "updated_at": now,
					}).Error; err != nil {
						return err
					}
				}
				code = activitytypes.CodeRunStarted
				if err := runtimeAdmissionMessageTxInternal(tx, now, row, c, code); err != nil {
					return err
				}
			}
		}
		snapshot, err := runtimeSnapshotTxInternal(tx, row)
		if err != nil {
			return err
		}
		result = RuntimeAdmissionResult{RuntimeAdmission: activitytypes.RuntimeAdmission{ActivityID: row.RunID, Code: code, Activity: snapshot}}
		found = true
		return nil
	})
	if err != nil {
		return RuntimeAdmissionResult{}, false, err
	}
	if found && result.Code == activitytypes.CodeRunStarted && s.localExecution != nil && c.command.Store == s.localExecution.Store {
		if runtimeQueuesInternal(c.command.Operation) {
			lock := s.activity.publishLockInternal(result.ActivityID)
			lock.Lock()
			s.activity.publishActivityInternal(result.Activity.Activity)
			lock.Unlock()
		}
		s.registerLocalInternal(ctx, c, result.ActivityID)
	}
	return result, found, nil
}

func runtimeActiveConflictTxInternal(tx *gorm.DB, c runtimeCanonicalCommandInternal) (*RuntimeReceipt, error) {
	target := c.command.Target
	// All directions are evaluated before selecting a join: a parent relationship
	// is always busy, never joinable, even if an exact row also exists.
	predicate := "gate_scope = ?"
	args := []any{c.gateScope}
	if target.Kind == "stack" {
		predicate += " OR (target_kind = 'container' AND parent_project_id = ?)"
		args = append(args, target.ID)
	} else if target.ParentProjectID != nil {
		predicate += " OR (target_kind = 'stack' AND target_id = ?)"
		args = append(args, *target.ParentProjectID)
	}
	var rows []RuntimeReceipt
	err := tx.Where("kind = 'receipt' AND settled_at IS NULL AND environment_id = ?", c.command.Store.EnvironmentID).
		Where("("+predicate+")", args...).Order("run_id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	var exact *RuntimeReceipt
	for i := range rows {
		row := &rows[i]
		identity := c.identity(row.RunID)
		identity.RequestDigest = row.RequestDigest // Different commands may conflict.
		if !runtimeMatchesInternal(row, identity) {
			return nil, &runtimeConflictInternal{}
		}
		if *row.GateScope != c.gateScope || *row.GateKey != c.gateKey {
			return nil, &GateConflict{RunID: row.RunID, Code: string(activitytypes.CodeRunBusy)}
		}
		if row.CancelRequested {
			return nil, &GateConflict{RunID: row.RunID, Code: string(activitytypes.CodeRunCancelling)}
		}
		exact = row
	}
	return exact, nil
}

func runtimeAdmissionMessageTxInternal(tx *gorm.DB, now time.Time, row *RuntimeReceipt, c runtimeCanonicalCommandInternal, code activitytypes.Code) error {
	// Bounded structured evidence carries identity/attribution only, never request
	// bodies, Compose text, environment contents or credentials.
	return tx.Create(&ActivityMessage{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: new(now)},
		ActivityID: row.RunID, Level: activitytypes.MessageLevelInfo, Code: string(code), Message: string(code),
		Payload: database.JSON{"actorId": c.command.Actor.ID, "key": c.command.Key, "operation": string(c.command.Operation)}}).Error
}
