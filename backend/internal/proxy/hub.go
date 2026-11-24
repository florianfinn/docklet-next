// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

package proxy

import (
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/modules"
	activitydto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/features"
	moduledto "github.com/florianfinn/docklet-next/types/modules"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/google/uuid"
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
	o := dto.Operation{Kind: dto.RouteOperation, ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, Digest: d, State: dto.Pending, Phase: dto.Admitted, Target: q.Target, Old: q.Expected, CreatedAt: time.Now().UTC()}
	stored := q
	stored.Kind = dto.RouteOperation
	request, err := marshal(stored)
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
			var row instanceRow
			er := tx.First(&row, "id = ?", q.Instance.ID).Error
			if er == nil {
				var stored dto.Instance
				if json.Unmarshal(row.Data, &stored) != nil || stored.Identity != q.Instance.Identity || stored.Kind != q.Instance.Kind || !sameSource(stored.Source, q.Instance.Source) {
					return dto.Fail(dto.ContainerChanged, 409)
				}
			} else if !missing(er) {
				return er
			}
			var held reservation
			er = tx.First(&held, "instance_id = ?", q.Instance.ID).Error
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
	if remote.Registration != q.Registration || remote.ID != q.ID || remote.InstanceID != q.Instance.ID || remote.Digest != prior.Digest {
		return h.recovery(ctx, prior, dto.OperationConflict)
	}
	if remote.Kind.Effective() != prior.Kind.Effective() || !remote.State.ValidFor(remote.Kind) {
		return h.recovery(ctx, prior, dto.JournalLost)
	}
	if err := h.persist(ctx, remote); err != nil {
		if p, ok := errors.AsType[*dto.Problem](err); ok {
			return h.recovery(ctx, prior, p.Code)
		}
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
		if o.Kind == dto.ResolverOperation && !(o.State == dto.Released && o.Reason == dto.UnknownOperationKind) {
			candidate := current
			candidate.State = o.State
			candidate.Result = b
			_, decoded, er := decodeRecord(candidate)
			if er != nil || decoded.Reason == dto.UnknownOperationKind {
				return dto.Fail(dto.JournalLost, 409)
			}
		}
		if err := adoptResolver(tx, current, o); err != nil {
			return err
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
			if request.Instance.EnvironmentID == "" {
				var row instanceRow
				if e := tx.First(&row, "id = ?", current.InstanceID).Error; e != nil {
					return e
				}
				request.Instance.EnvironmentID = row.EnvironmentID
			}
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
	if o.State.Final() || deferredOperation(o) {
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
	if deferredOperation(o) && q.Instance.EnvironmentID == "" {
		var instance instanceRow
		if err := h.DB.WithContext(ctx).First(&instance, "id = ?", r.InstanceID).Error; err != nil {
			return o, err
		}
		if err := json.Unmarshal(instance.Data, &q.Instance); err != nil {
			return o, dto.Fail(dto.UnknownOperationKind, 409)
		}
	}
	q.ID, q.Generation, q.Registration = r.ID, r.Generation, r.Registration
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

func (h *Hub) CreateResolver(ctx context.Context, q dto.ResolverApplyRequest) (dto.Operation, error) {
	if err := validResolver(q); err != nil {
		return dto.Operation{}, err
	}
	en := resolverEnvelope(q)
	digest, err := en.Digest()
	if err != nil {
		return dto.Operation{}, err
	}
	h.mu.Lock()
	var existing Record
	err = h.DB.WithContext(ctx).First(&existing, "id = ?", q.ID).Error
	if err == nil {
		h.mu.Unlock()
		if existing.Digest != digest {
			return dto.Operation{}, dto.Fail(dto.OperationConflict, 409)
		}
		return h.Query(ctx, q.ID)
	}
	if !missing(err) {
		h.mu.Unlock()
		return dto.Operation{}, err
	}
	reg, err := h.Transport.Registration(ctx, q.Instance.EnvironmentID)
	if err != nil {
		h.mu.Unlock()
		return dto.Operation{}, err
	}
	if reg != q.Registration {
		h.mu.Unlock()
		return dto.Operation{}, dto.Fail(dto.RegistrationChanged, 409)
	}
	o := dto.Operation{Kind: dto.ResolverOperation, ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, Digest: digest, State: dto.Pending, Phase: dto.Admitted, CreatedAt: time.Now().UTC(), Resolver: &dto.ResolverReceipt{Intent: en.Intent, Phase: dto.ResolverAdmitted, Files: []dto.ResolverFile{}}}
	request, _ := marshal(en)
	result, _ := marshal(o)
	err = h.Modules.Admit(ctx, features.Proxy, func(ctx context.Context) error {
		return h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var row instanceRow
			er := tx.First(&row, "id = ?", q.Instance.ID).Error
			if er == nil {
				var stored dto.Instance
				if json.Unmarshal(row.Data, &stored) != nil || stored.Identity != q.Instance.Identity || stored.Kind != q.Instance.Kind || !sameSource(stored.Source, q.Instance.Source) {
					return dto.Fail(dto.ContainerChanged, 409)
				}
			} else if !missing(er) {
				return er
			}
			var held reservation
			er = tx.First(&held, "instance_id = ?", q.Instance.ID).Error
			if er == nil {
				return &dto.Problem{Status: 409, Code: dto.ModuleBusy, OperationID: held.OperationID}
			}
			if !missing(er) {
				return er
			}
			if er = tx.Create(&Record{ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, Digest: digest, State: o.State, Request: request, Result: result, CreatedAt: o.CreatedAt}).Error; er != nil {
				return er
			}
			return tx.Create(&reservation{InstanceID: q.Instance.ID, OperationID: q.ID}).Error
		})
	})
	h.mu.Unlock()
	if err != nil {
		return o, err
	}
	ctx = context.WithoutCancel(ctx)
	sender, ok := h.Transport.(interface {
		CreateResolver(context.Context, dto.ResolverApplyRequest) (dto.Operation, error)
	})
	if !ok {
		return h.query(ctx, q.ID, dto.EngineUnavailable)
	}
	remote, err := sender.CreateResolver(ctx, q)
	if err != nil {
		return h.query(ctx, q.ID, reason(err, dto.AgentUnavailable))
	}
	binding := dto.ApplyRequest{Kind: dto.ResolverOperation, ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration}
	return h.accept(ctx, binding, o, remote)
}

func (m *Module) CreateResolver(ctx context.Context, id string, input dto.ResolverRequest) (dto.Operation, error) {
	if err := input.Validate(); err != nil {
		return dto.Operation{}, err
	}
	i, err := m.Instance(ctx, id)
	if err != nil {
		return dto.Operation{}, err
	}
	if i.EnvironmentID != "0" {
		if m.Transport == nil || m.Transport.Hosts == nil {
			return dto.Operation{}, dto.Fail(dto.AgentUnavailable, 503)
		}
		why := m.Transport.Hosts.ModuleReason(ctx, i.EnvironmentID, features.Proxy)
		if why == "agent.update-required" {
			return dto.Operation{}, &moduledto.Problem{Status: 409, Code: why, Feature: features.Proxy}
		}
		if why != "" {
			return dto.Operation{}, dto.Fail(dto.AgentUnavailable, 503)
		}
	}
	i, err = m.writeAdmission(ctx, id, false)
	if err != nil {
		return dto.Operation{}, err
	}
	return m.Hub.CreateResolver(ctx, dto.ResolverApplyRequest{ID: uuid.NewString(), Instance: i, Generation: i.Generation, Registration: i.Registration, Request: input})
}

// adoptResolver runs in the receipt transaction. A concurrent re-registration
// cannot be overwritten; a repeated receipt for the adopted identity is a no-op.
func adoptResolver(tx *gorm.DB, record Record, o dto.Operation) error {
	if o.Kind != dto.ResolverOperation || (o.State != dto.Applied && o.State != dto.RolledBack) {
		return nil
	}
	q, _, err := decodeRecord(record)
	if err != nil {
		return err
	}
	if o.Resolver == nil {
		return dto.Fail(dto.JournalLost, 409)
	}
	replacement := o.Resolver.Replacement
	if replacement == nil {
		if o.State == dto.RolledBack && !o.Resolver.RecreateCommitted {
			return nil
		}
		return dto.Fail(dto.IdentityUnclear, 409)
	}
	if replacement.ContainerID == "" || replacement.Name == "" || replacement.StartedAt == "" || replacement.Canonical != q.Instance.Source.Canonical || replacement.MountTarget != q.Instance.Source.MountTarget || replacement.Kind != q.Instance.Source.Kind {
		return dto.Fail(dto.IdentityUnclear, 409)
	}
	var row instanceRow
	if err := tx.First(&row, "id = ?", record.InstanceID).Error; err != nil {
		return err
	}
	// A changed instance row skips adoption: the final receipt and reservation
	// release still persist, and the instance shows container-changed as in M27.
	var current dto.Instance
	if json.Unmarshal(row.Data, &current) != nil {
		return nil
	}
	if current.Registration != q.Registration || current.Source.Canonical != replacement.Canonical || current.Source.MountTarget != replacement.MountTarget || current.Source.Kind != replacement.Kind {
		return nil
	}
	if current.Identity.ContainerID == replacement.ContainerID {
		return nil
	}
	if current.Identity.ContainerID != q.Instance.Identity.ContainerID {
		return nil
	}
	current.Identity = dto.Identity{ContainerID: replacement.ContainerID, ContainerName: replacement.Name, StartedAt: replacement.StartedAt}
	data, err := marshal(current)
	if err != nil {
		return err
	}
	return tx.Model(&instanceRow{}).Where("id = ? AND data = ?", record.InstanceID, row.Data).Update("data", data).Error
}
