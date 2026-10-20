// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Durable trusted registration; binding changes never release held work.

package activity

import (
	"context"
	"errors"
	"sync"
	"time"

	"encoding/json/v2"
	"github.com/danielgtaylor/huma/v2"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
	"io"
)

type RuntimeExecutorBinding struct {
	ID             int    `gorm:"column:id;primaryKey;autoIncrement:false;not null"`
	ManagerID      string `gorm:"column:manager_id;not null"`
	RegistrationID string `gorm:"column:registration_id;not null"`
}

func (RuntimeExecutorBinding) TableName() string { return "runtime_executor_binding" }

// LocalBinding is a startup-only read before workers/readiness. An unenrolled
// agent uses its own persisted instance until authenticated registration occurs.
func (s *RuntimeService) LocalBinding(ctx context.Context, generation, instance string) (RuntimeStoreBinding, error) {
	binding := RuntimeStoreBinding{EnvironmentID: "0", StoreGeneration: generation, ExecutorGeneration: generation, ManagerID: instance, RegistrationID: instance}
	var row RuntimeExecutorBinding
	err := s.activity.db.WithContext(ctx).First(&row, "id = 1").Error
	if err == nil {
		binding.ManagerID, binding.RegistrationID = row.ManagerID, row.RegistrationID
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return RuntimeStoreBinding{}, &RuntimeStoreError{Cause: err}
	}
	return binding, nil
}

// RuntimeReceiverBinding serializes trusted registration changes against every
// execution/query/cancel request, in addition to the shared owner/SQLite locks.
type runtimeReceiverBindingInternal struct{ sync.RWMutex }

func (r *RuntimeReceiver) Register(ctx context.Context, request activitytypes.RuntimeRegistration) (activitytypes.RuntimeExecutorIdentity, error) {
	if !runtimeTextInternal(request.ManagerID, 256) || !runtimeTextInternal(request.RegistrationID, 256) {
		return activitytypes.RuntimeExecutorIdentity{}, errRuntimeInvalidInternal
	}
	r.binding.Lock()
	defer r.binding.Unlock()
	s := r.service
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	next := r.store
	next.ManagerID, next.RegistrationID = request.ManagerID, request.RegistrationID
	err := s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		if err := runtimeCheckStoreTxInternal(tx, runtimeIdentityInternal{StoreGeneration: next.StoreGeneration, ExecutorGeneration: next.ExecutorGeneration, ManagerID: next.ManagerID, RegistrationID: next.RegistrationID, EnvironmentID: "0", RequestDigest: "metadata"}); err != nil {
			return err
		}
		if next != r.store {
			if len(s.owners) != 0 {
				return &GateConflict{Code: string(activitytypes.CodeRunBusy)}
			}
			var held int64
			if err := tx.Model(&RuntimeReceipt{}).Where("environment_id = '0' AND kind = 'receipt' AND settled_at IS NULL").Count(&held).Error; err != nil {
				return err
			}
			if held != 0 {
				return &GateConflict{Code: string(activitytypes.CodeRunBusy)}
			}
		}
		var existing RuntimeExecutorBinding
		err := tx.First(&existing, "id = 1").Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return runtimeReceiverCreateTxInternal(tx, &RuntimeExecutorBinding{ID: 1, ManagerID: next.ManagerID, RegistrationID: next.RegistrationID})
		}
		if err != nil {
			return err
		}
		if existing.ManagerID == next.ManagerID && existing.RegistrationID == next.RegistrationID {
			return nil
		}
		updated := tx.Model(&RuntimeExecutorBinding{}).Where("id = 1 AND manager_id = ? AND registration_id = ?", existing.ManagerID, existing.RegistrationID).UpdateColumns(map[string]any{"manager_id": next.ManagerID, "registration_id": next.RegistrationID})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return &runtimeConflictInternal{}
		}
		return nil
	})
	if err != nil {
		return activitytypes.RuntimeExecutorIdentity{}, err
	}
	r.store, s.localExecution.Store = next, next
	return activitytypes.RuntimeExecutorIdentity{ManagerID: next.ManagerID, RegistrationID: next.RegistrationID, ExecutorGeneration: next.ExecutorGeneration}, nil
}

func runtimeMetadataHTTPInternal(receiver *RuntimeReceiver, ctx huma.Context) {
	body, err := io.ReadAll(io.LimitReader(ctx.BodyReader(), activitytypes.RuntimeMaxBodyBytes+1))
	if err != nil || len(body) > activitytypes.RuntimeMaxBodyBytes || ctx.URL().RawQuery != "" {
		runtimeHTTPErrorInternal(ctx, 400, "Invalid runtime metadata request")
		return
	}
	var result any
	if ctx.Operation().OperationID == "register-runtime-executor" {
		var request activitytypes.RuntimeRegistration
		if activitytypes.DecodeRuntimeJSON(body, &request) != nil {
			runtimeHTTPErrorInternal(ctx, 400, "Invalid runtime registration")
			return
		}
		result, err = receiver.Register(ctx.Context(), request)
	} else {
		var request activitytypes.RuntimeTargetRequest
		if activitytypes.DecodeRuntimeJSON(body, &request) != nil {
			runtimeHTTPErrorInternal(ctx, 400, "Invalid runtime target")
			return
		}
		result, err = receiver.Resolve(ctx.Context(), request)
	}
	if err != nil {
		var mismatch *runtimeConflictInternal
		var busy *GateConflict
		if errors.As(err, &mismatch) || errors.As(err, &busy) {
			runtimeHTTPErrorInternal(ctx, 409, "Runtime registration conflict")
		} else {
			runtimeHTTPErrorInternal(ctx, 503, "Runtime metadata unavailable")
		}
		return
	}
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(200)
	_ = json.MarshalWrite(ctx.BodyWriter(), result)
}

func (r *RuntimeReceiver) Resolve(ctx context.Context, request activitytypes.RuntimeTargetRequest) (activitytypes.RuntimeTargetMetadata, error) {
	r.binding.RLock()
	defer r.binding.RUnlock()
	if request.Registration.ManagerID != r.store.ManagerID || request.Registration.RegistrationID != r.store.RegistrationID || request.Registration.ExecutorGeneration != r.store.ExecutorGeneration {
		return activitytypes.RuntimeTargetMetadata{}, &runtimeConflictInternal{}
	}
	if !runtimeTargetValidInternal(request.Target) || (request.Target.Kind != "stack" && request.Target.Kind != "container") {
		return activitytypes.RuntimeTargetMetadata{}, errRuntimeInvalidInternal
	}
	resolved, err := r.service.resolver.ResolveRuntimeTarget(ctx, request.Target)
	if err != nil {
		return activitytypes.RuntimeTargetMetadata{}, err
	}
	return activitytypes.RuntimeTargetMetadata{Target: resolved.Target, ComposeManaged: resolved.ComposeManaged}, nil
}
