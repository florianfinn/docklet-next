// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant trusted receiver core and actual snapshots; HTTP and sequenced detail remain separate.

package activity

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RuntimeReceiver shares the sole local RuntimeService and its lifetime owner.
// Every entry requires an authenticated manager bound to this immutable trusted
// construction. Wire identity and attribution never authenticate a caller.
type RuntimeReceiver struct {
	service *RuntimeService
	store   RuntimeStoreBinding
}

func NewRuntimeReceiver(service *RuntimeService) (*RuntimeReceiver, error) {
	if err := service.checkInternal(); err != nil {
		return nil, err
	}
	local := service.localExecution
	if local == nil || local.ApplicationContext == nil || local.Executor == nil {
		return nil, errRuntimeInvalidInternal
	}
	store := local.Store
	generation, err := uuid.Parse(store.StoreGeneration)
	if err != nil || generation.Version() != 4 || generation.String() != store.StoreGeneration ||
		store.EnvironmentID != "0" || store.ExecutorGeneration != store.StoreGeneration ||
		!runtimeTextInternal(store.ManagerID, 256) || !runtimeTextInternal(store.RegistrationID, 256) {
		return nil, errRuntimeInvalidInternal
	}
	return &RuntimeReceiver{service: service, store: store}, nil
}

// RuntimeReceiverResult is receipt-backed internal evidence, not a complete wire
// outcome. C5 must capture actual bounded output/messages/sequence windows before
// constructing RuntimeExecutionDetail. A committed refusal fence has nil Snapshot.
type RuntimeReceiverResult struct {
	Identity activitytypes.RuntimeRunIdentity
	State    activitytypes.RuntimeAnswerState
	Snapshot *activitytypes.RuntimeSnapshot
}

func (r *RuntimeReceiver) identityInternal(wire activitytypes.RuntimeRunIdentity) (runtimeIdentityInternal, error) {
	if r == nil || r.service == nil {
		return runtimeIdentityInternal{}, errRuntimeInvalidInternal
	}
	if err := r.service.checkInternal(); err != nil {
		return runtimeIdentityInternal{}, err
	}
	// Authenticated tuple comparison precedes all command conversion/normalization.
	if wire.ManagerID != r.store.ManagerID || wire.RegistrationID != r.store.RegistrationID ||
		wire.ExecutorGeneration != r.store.ExecutorGeneration {
		return runtimeIdentityInternal{}, &runtimeConflictInternal{}
	}
	runID, err := runtimeUUIDInternal(wire.RunID)
	digest, digestErr := hex.DecodeString(wire.RequestDigest)
	if err != nil || runID != wire.RunID || digestErr != nil || len(digest) != 32 || strings.ToLower(wire.RequestDigest) != wire.RequestDigest {
		return runtimeIdentityInternal{}, errRuntimeInvalidInternal
	}
	return runtimeIdentityInternal{RunID: runID, StoreGeneration: r.store.StoreGeneration, ManagerID: r.store.ManagerID,
		RegistrationID: r.store.RegistrationID, EnvironmentID: "0", ExecutorGeneration: r.store.ExecutorGeneration, RequestDigest: wire.RequestDigest}, nil
}

func runtimeReceiverResultTxInternal(tx *gorm.DB, row *RuntimeReceipt) (RuntimeReceiverResult, error) {
	result := RuntimeReceiverResult{Identity: activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{
		ManagerID: row.ManagerID, RegistrationID: row.RegistrationID, ExecutorGeneration: row.ExecutorGeneration}, RunID: row.RunID, RequestDigest: row.RequestDigest},
		State: activitytypes.RuntimeAnswerNotReceived}
	if row.Kind == "not_received" {
		return result, nil
	}
	snapshot, err := runtimeSnapshotTxInternal(tx, row)
	if err != nil {
		return RuntimeReceiverResult{}, err
	}
	result.State, result.Snapshot = activitytypes.RuntimeAnswerActive, &snapshot
	if row.SettledAt != nil {
		result.State = activitytypes.RuntimeAnswerTerminal
	}
	return result, nil
}

// Execute requires the authenticated manager binding described on RuntimeReceiver.
// It preserves the request context through acceptance; only a successful commit
// hands work to the existing application lifetime, even after caller disconnect.
func (r *RuntimeReceiver) Execute(ctx context.Context, envelope activitytypes.RuntimeExecutionEnvelope) (RuntimeReceiverResult, error) {
	id, err := r.identityInternal(envelope.Identity)
	if err != nil {
		return RuntimeReceiverResult{}, err
	}
	c, err := runtimeCanonicalizeExecutorInternal(RuntimeCommand{Store: r.store, ProposedRunID: id.RunID,
		Actor:     usertypes.Actor{ID: envelope.Attribution.ID, Username: envelope.Attribution.Username, DisplayName: envelope.Attribution.DisplayName},
		Operation: envelope.Operation, Target: envelope.Target, Options: envelope.Options}, id.RequestDigest)
	if err != nil {
		return RuntimeReceiverResult{}, err
	}
	result, found, err := r.receivePassInternal(ctx, c, false)
	if err != nil || found {
		return result, err
	}
	s := r.service
	if s.resolver == nil {
		return RuntimeReceiverResult{}, &RuntimeTargetError{Code: "runtime.target-unavailable"}
	}
	target := c.command.Target
	target.Name, target.ParentProjectID = copyPtrInternal(target.Name), copyPtrInternal(target.ParentProjectID)
	resolved, err := s.resolver.ResolveRuntimeTarget(ctx, target)
	if ctx.Err() != nil {
		return RuntimeReceiverResult{}, ctx.Err()
	}
	if err != nil {
		return RuntimeReceiverResult{}, &RuntimeTargetError{Code: "runtime.target-unavailable", Cause: err}
	}
	if resolved.ComposeManaged && resolved.Target.ParentProjectID == nil {
		return RuntimeReceiverResult{}, &RuntimeTargetError{Code: string(activitytypes.CodeRuntimeParentUnresolved)}
	}
	if !runtimeTargetValidInternal(resolved.Target) || resolved.Target.Kind != target.Kind || resolved.Target.ID != target.ID ||
		!runtimeEqualInternal(resolved.Target.ParentProjectID, c.command.Target.ParentProjectID) {
		return RuntimeReceiverResult{}, &runtimeConflictInternal{}
	}
	c.command.Target.Name = copyPtrInternal(resolved.Target.Name)
	result, _, err = r.receivePassInternal(ctx, c, true)
	return result, err
}

func (r *RuntimeReceiver) receivePassInternal(ctx context.Context, c runtimeCanonicalCommandInternal, fresh bool) (RuntimeReceiverResult, bool, error) {
	s := r.service
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	var result RuntimeReceiverResult
	found, created := false, false
	err := s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		row, err := runtimeReadTxInternal(tx, c.identity(c.command.ProposedRunID))
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if !fresh {
				return nil
			}
			row, err = runtimeReceiveTxInternal(tx, now, c)
			if err != nil {
				return err
			}
			created = true
		} else if row.Kind != "receipt" {
			return &runtimeConflictInternal{}
		}
		result, err = runtimeReceiverResultTxInternal(tx, row)
		found = err == nil
		return err
	})
	if err != nil {
		return RuntimeReceiverResult{}, false, err
	}
	if created {
		if runtimeQueuesInternal(c.command.Operation) {
			lock := s.activity.publishLockInternal(result.Identity.RunID)
			lock.Lock()
			s.activity.publishActivityInternal(result.Snapshot.Activity)
			lock.Unlock()
		}
		s.registerLocalInternal(ctx, c, result.Identity.RunID)
	}
	return result, found, nil
}

// Query requires authenticated manager binding. Absence is returned only after
// runtimeQueryTxInternal's matching tombstone commits in the receipt transaction.
func (r *RuntimeReceiver) Query(ctx context.Context, wire activitytypes.RuntimeRunIdentity) (RuntimeReceiverResult, error) {
	id, err := r.identityInternal(wire)
	if err != nil {
		return RuntimeReceiverResult{}, err
	}
	s := r.service
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	var result RuntimeReceiverResult
	err = s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		row, err := runtimeQueryTxInternal(tx, now, id)
		if err != nil {
			return err
		}
		result, err = runtimeReceiverResultTxInternal(tx, row)
		return err
	})
	if err != nil {
		return RuntimeReceiverResult{}, err
	}
	return result, nil
}

// Cancel requires authenticated manager binding and bounded audit attribution.
// Compose the same durable intent-before-signal contract as RuntimeService.Cancel,
// matching the full digest inside its intent transaction. No unchecked reference
// or nested runningMu acquisition can introduce a validation/cancellation race.
func (r *RuntimeReceiver) Cancel(ctx context.Context, wire activitytypes.RuntimeRunIdentity, attribution activitytypes.RuntimeAttribution) (RuntimeReceiverResult, error) {
	id, err := r.identityInternal(wire)
	if err != nil {
		return RuntimeReceiverResult{}, err
	}
	// Only bounded attribution is consumed; it cannot grant read/cancel authority.
	if !runtimeTextInternal(attribution.ID, 256) || !utf8.ValidString(attribution.Username) || len(attribution.Username) > 256 ||
		strings.ContainsFunc(attribution.Username, unicode.IsControl) || (attribution.DisplayName != nil &&
		(!utf8.ValidString(*attribution.DisplayName) || len(*attribution.DisplayName) > 256 || strings.ContainsFunc(*attribution.DisplayName, unicode.IsControl))) {
		return RuntimeReceiverResult{}, errRuntimeInvalidInternal
	}
	s := r.service
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	lock := s.activity.publishLockInternal(id.RunID)
	lock.Lock()
	defer lock.Unlock()
	var result RuntimeReceiverResult
	var message *ActivityMessage
	var row *RuntimeReceipt
	err = s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		var err error
		row, err = runtimeMutableReceiptTxInternal(tx, id)
		if err != nil {
			return err
		}
		if row.SettledAt == nil && !row.CancelRequested {
			row, err = runtimeCancelIntentTxInternal(tx, now, id)
			if err != nil {
				return err
			}
			message = &ActivityMessage{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: new(now)},
				ActivityID: id.RunID, Level: activitytypes.MessageLevelWarning, Code: string(activitytypes.CodeRunCancelling),
				Message: "Runtime cancellation requested", Payload: database.JSON{"actorId": attribution.ID, "operation": *row.OperationID}}
			if err := tx.Create(message).Error; err != nil {
				return err
			}
		}
		result, err = runtimeReceiverResultTxInternal(tx, row)
		return err
	})
	if err != nil {
		return RuntimeReceiverResult{}, err
	}
	if row.SettledAt == nil && row.CancelRequested {
		if owner := s.owners[id.RunID]; owner != nil {
			owner.cancelRequested = true
			owner.cancel(activitylib.ErrCanceled)
		}
	}
	if message != nil {
		s.activity.publishMessageInternal(result.Snapshot.EnvironmentID, result.Snapshot.Type, activityMessageToDTOInternal(message))
	}
	return result, nil
}
