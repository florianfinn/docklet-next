// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

package proxy

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/modules"
	activitydto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"gorm.io/gorm"
	"sync"
	"time"
)

type Transport interface {
	Registration(context.Context, string) (string, error)
	Apply(context.Context, dto.ApplyRequest) (dto.Operation, error)
	Query(context.Context, dto.Query) (dto.Operation, error)
	Fence(context.Context, dto.Query) (dto.FenceResult, error)
	Release(context.Context, dto.ReleaseRequest) (dto.Operation, error)
}
type Hub struct {
	DB        *gorm.DB
	Modules   *modules.Service
	Transport Transport
	mu        sync.Mutex
}

func (h *Hub) Apply(ctx context.Context, q dto.ApplyRequest) (dto.Operation, error) {
	if err := validRequest(q); err != nil {
		return dto.Operation{}, err
	}
	d, err := Digest(q)
	if err != nil {
		return dto.Operation{}, err
	}
	h.mu.Lock()
	var existing Record
	err = h.DB.WithContext(ctx).First(&existing, "id = ?", q.ID).Error
	if err == nil {
		h.mu.Unlock()
		if existing.Digest != d {
			return dto.Operation{}, dto.Fail(dto.OperationConflict, 409)
		}
		return h.Query(ctx, q.ID)
	}
	if !missing(err) {
		h.mu.Unlock()
		return dto.Operation{}, err
	}
	current, err := h.Transport.Registration(ctx, q.Instance.EnvironmentID)
	if err != nil {
		h.mu.Unlock()
		return dto.Operation{}, err
	}
	if current != q.Registration {
		h.mu.Unlock()
		return dto.Operation{}, dto.Fail(dto.RegistrationChanged, 409)
	}
	o := dto.Operation{ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, Digest: d, State: dto.Pending, Phase: dto.Admitted, Target: q.Target, Old: q.Expected, CreatedAt: time.Now().UTC()}
	request, err := marshal(q)
	if err != nil {
		h.mu.Unlock()
		return o, err
	}
	result, err := marshal(o)
	if err != nil {
		h.mu.Unlock()
		return o, err
	}
	err = h.Modules.Admit(ctx, features.Proxy, func(ctx context.Context) error {
		return h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var held reservation
			er := tx.First(&held, "instance_id = ?", q.Instance.ID).Error
			if er == nil {
				return &dto.Problem{Status: 409, Code: dto.ModuleBusy, OperationID: held.OperationID}
			}
			if !missing(er) {
				return er
			}
			if er = tx.Create(&Record{ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Digest: d, Registration: q.Registration, State: dto.Pending, Request: request, Result: result, CreatedAt: o.CreatedAt}).Error; er != nil {
				return er
			}
			return tx.Create(&reservation{InstanceID: q.Instance.ID, OperationID: q.ID}).Error
		})
	})
	h.mu.Unlock()
	if err != nil {
		return o, err
	}
	// This is the only Apply dispatch. Persisted records, including those created
	// just before a hub crash, are followed exclusively by Query and Fence. The
	// dispatch and its follow-up outlive the caller; the reconciler finishes the rest.
	ctx = context.WithoutCancel(ctx)
	remote, err := h.Transport.Apply(ctx, q)
	if err != nil {
		refusal := dto.Reason("")
		if p, ok := errors.AsType[*dto.Problem](err); ok {
			refusal = p.Code
		}
		return h.query(ctx, q.ID, refusal)
	}
	return h.accept(ctx, q, o, remote)
}
func (h *Hub) accept(ctx context.Context, q dto.ApplyRequest, prior, remote dto.Operation) (dto.Operation, error) {
	reg, err := h.Transport.Registration(ctx, q.Instance.EnvironmentID)
	if err != nil {
		return prior, err
	}
	if reg != q.Registration {
		return h.recovery(ctx, prior, dto.RegistrationChanged)
	}
	if remote.Generation != q.Generation {
		return h.recovery(ctx, prior, dto.GenerationMismatch)
	}
	if remote.Registration != q.Registration || remote.ID != q.ID || remote.Digest != prior.Digest {
		return h.recovery(ctx, prior, dto.OperationConflict)
	}
	if !remote.State.Valid() {
		return h.recovery(ctx, prior, dto.JournalLost)
	}
	if err := h.persist(ctx, remote); err != nil {
		return prior, err
	}
	var row Record
	if err := h.DB.WithContext(ctx).First(&row, "id = ?", q.ID).Error; err != nil {
		return prior, err
	}
	_, recorded, err := decodeRecord(row)
	return recorded, err
}
func (h *Hub) persist(ctx context.Context, o dto.Operation) error {
	b, err := marshal(o)
	if err != nil {
		return err
	}
	return h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Record
		if err := tx.First(&current, "id = ?", o.ID).Error; err != nil {
			return err
		}
		if current.State.Final() {
			return nil
		}
		if o.State.Final() && o.FinishedAt == nil {
			now := time.Now().UTC()
			o.FinishedAt = &now
			b, err = marshal(o)
			if err != nil {
				return err
			}
		}
		res := tx.Model(&Record{}).Where("id = ? AND registration = ? AND generation = ?", o.ID, o.Registration, o.Generation).Updates(map[string]any{"state": o.State, "reason": o.Reason, "evidence": o.Evidence, "actor": o.ReleasedBy, "result": b, "finished_at": o.FinishedAt})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return dto.Fail(dto.OperationConflict, 409)
		}
		if o.State == dto.Released {
			request, _, er := decodeRecord(current)
			if er != nil {
				return er
			}
			resource := "proxy"
			code := string(activitydto.CodeProxyReleased)
			dedup := "proxy.release:" + o.ID
			metadata := database.JSON{"operationId": o.ID, "instanceId": o.InstanceID, "oldHash": o.Old.Hash, "newHash": o.New.Hash, "previousReason": current.Reason}
			if o.Current != nil {
				metadata["currentPresent"] = o.Current.Present
				metadata["currentHash"] = o.Current.Hash
			}
			if er = tx.Create(&event.Event{DeduplicationKey: &dedup, Type: event.EventType(code), MessageCode: code, Severity: event.EventSeverityWarning, Title: code, ResourceType: &resource, ResourceID: &o.InstanceID, UserID: &o.ReleasedBy, EnvironmentID: &request.Instance.EnvironmentID, Metadata: metadata, Timestamp: time.Now().UTC()}).Error; er != nil {
				return er
			}
		}
		if o.State.Final() {
			return tx.Delete(&reservation{}, "operation_id = ?", o.ID).Error
		}
		return nil
	})
}
func (h *Hub) recovery(ctx context.Context, o dto.Operation, r dto.Reason) (dto.Operation, error) {
	o.State = dto.RecoveryRequired
	o.Reason = r
	o.FinishedAt = nil
	return o, h.persist(ctx, o)
}
func (h *Hub) Query(ctx context.Context, id string) (dto.Operation, error) {
	return h.query(ctx, id, "")
}

// query follows one record; refusal is the agent's typed answer to the lost
// dispatch, kept as the reason when the fence proves nothing was admitted.
func (h *Hub) query(ctx context.Context, id string, refusal dto.Reason) (dto.Operation, error) {
	var row Record
	if err := h.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return dto.Operation{}, err
	}
	q, o, err := decodeRecord(row)
	if err != nil {
		return o, err
	}
	if o.State.Final() {
		return o, nil
	}
	reg, err := h.Transport.Registration(ctx, q.Instance.EnvironmentID)
	if err != nil {
		return o, err
	}
	if reg != q.Registration {
		return h.recovery(ctx, o, dto.RegistrationChanged)
	}
	query := dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration}
	remote, err := h.Transport.Query(ctx, query)
	if err == nil {
		return h.accept(ctx, q, o, remote)
	}
	p, ok := errors.AsType[*dto.Problem](err)
	if !ok {
		return o, err
	}
	if p.Code == dto.GenerationMismatch || p.Code == dto.JournalLost {
		return h.recovery(ctx, o, p.Code)
	}
	if p.Code != dto.UnknownOperation {
		return o, err
	}
	fenced, err := h.Transport.Fence(ctx, query)
	if err != nil {
		if p, ok = errors.AsType[*dto.Problem](err); ok && (p.Code == dto.GenerationMismatch || p.Code == dto.JournalLost) {
			return h.recovery(ctx, o, p.Code)
		}
		return o, err
	}
	if fenced.Generation != q.Generation {
		return h.recovery(ctx, o, dto.GenerationMismatch)
	}
	if fenced.Operation != nil {
		return h.accept(ctx, q, o, *fenced.Operation)
	}
	if !fenced.Fenced {
		return h.recovery(ctx, o, dto.JournalLost)
	}
	// Recheck registration after the fence too: no previous registration can settle.
	reg, err = h.Transport.Registration(ctx, q.Instance.EnvironmentID)
	if err != nil {
		return o, err
	}
	if reg != q.Registration {
		return h.recovery(ctx, o, dto.RegistrationChanged)
	}
	o.State = dto.NotApplied
	o.Reason = refusal
	now := time.Now().UTC()
	o.FinishedAt = &now
	return o, h.persist(ctx, o)
}
func (h *Hub) Release(ctx context.Context, id string, confirmed dto.FileState, actor string) (dto.Operation, error) {
	var r Record
	if err := h.DB.WithContext(ctx).First(&r, "id = ?", id).Error; err != nil {
		return dto.Operation{}, err
	}
	q, o, err := decodeRecord(r)
	if err != nil {
		return o, err
	}
	if o.State != dto.RecoveryRequired {
		return o, dto.Fail(dto.OperationConflict, 409)
	}
	targetRegistration, err := h.Transport.Registration(ctx, q.Instance.EnvironmentID)
	if err != nil {
		return o, err
	}
	remote, err := h.Transport.Release(ctx, dto.ReleaseRequest{Query: dto.Query{ID: id, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration}, Confirmed: confirmed, Actor: actor})
	if err != nil {
		// Both mean the agent holds no lock for this operation any more.
		p, ok := errors.AsType[*dto.Problem](err)
		if !ok || p.Code != dto.GenerationMismatch && p.Code != dto.JournalLost {
			return o, err
		}
		currentRegistration, registrationErr := h.Transport.Registration(ctx, q.Instance.EnvironmentID)
		if registrationErr != nil {
			return o, registrationErr
		}
		if currentRegistration != targetRegistration {
			return h.recovery(ctx, o, dto.RegistrationChanged)
		}
		o.State = dto.Released
		o.Current = &confirmed
		o.ReleasedBy = actor
		now := time.Now().UTC()
		o.FinishedAt = &now
		return o, h.persist(ctx, o)
	}
	// The agent answers Release with its final receipt: released, or the outcome
	// it reached before the hub lost track; either is adopted.
	if !remote.State.Final() {
		return o, dto.Fail(dto.OperationConflict, 409)
	}
	currentRegistration, err := h.Transport.Registration(ctx, q.Instance.EnvironmentID)
	if err != nil {
		return o, err
	}
	if currentRegistration != targetRegistration {
		return h.recovery(ctx, o, dto.RegistrationChanged)
	}
	if remote.ID != q.ID || remote.Generation != q.Generation || remote.Registration != q.Registration || remote.Digest != o.Digest {
		return o, dto.Fail(dto.OperationConflict, 409)
	}
	return remote, h.persist(ctx, remote)
}
func (h *Hub) Unfinished(ctx context.Context) (string, error) {
	return unfinished(ctx, h.DB, "proxy_operations")
}
