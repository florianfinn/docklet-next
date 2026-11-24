// Source: docklet-next/issues/592
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original hub-owned identical-source adoption and atomic receipt persistence.

package proxy

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/types/features"
	moduledto "github.com/florianfinn/docklet-next/types/modules"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// sameSource binds the actual mount and both proxy access and writer authority.
func sameSource(a, b dto.Source) bool {
	return a.MountTarget == b.MountTarget && a.Canonical == b.Canonical && a.Kind == b.Kind && a.MountMode == b.MountMode && a.ReadOnly == b.ReadOnly
}

func adoptionCandidate(i dto.Instance, rows []dto.Discovery) (dto.Identity, dto.AdoptionReason) {
	var candidates []dto.Discovery
	for _, d := range rows {
		if d.Identity.ContainerName == i.Identity.ContainerName {
			candidates = append(candidates, d)
		}
	}
	if len(candidates) == 0 {
		for _, d := range rows {
			if d.Identity.ContainerID == i.Identity.ContainerID {
				return dto.Identity{}, dto.AdoptionNameChanged
			}
		}
		return dto.Identity{}, dto.AdoptionContainerMissing
	}
	if len(candidates) != 1 {
		return dto.Identity{}, dto.AdoptionAmbiguous
	}
	d := candidates[0]
	// The stored container itself without a readable state (Epoch's container-changed) is not a recreation.
	if d.Identity.ContainerID == i.Identity.ContainerID {
		return dto.Identity{}, dto.AdoptionContainerMissing
	}
	if d.Kind != i.Kind {
		return dto.Identity{}, dto.AdoptionKindChanged
	}
	var sources []dto.Source
	for _, s := range d.Sources {
		if s.MountTarget == i.Source.MountTarget {
			sources = append(sources, s)
		}
	}
	if len(sources) > 1 {
		return dto.Identity{}, dto.AdoptionAmbiguous
	}
	if len(sources) == 0 {
		return dto.Identity{}, dto.AdoptionSourceChanged
	}
	s := sources[0]
	if s.Kind != i.Source.Kind {
		return dto.Identity{}, dto.AdoptionKindChanged
	}
	if s.MountMode != i.Source.MountMode || s.ReadOnly != i.Source.ReadOnly {
		return dto.Identity{}, dto.AdoptionModeChanged
	}
	if !sameSource(i.Source, s) {
		return dto.Identity{}, dto.AdoptionSourceChanged
	}
	if d.Identity.ContainerID == "" || d.Identity.StartedAt == "" {
		return dto.Identity{}, dto.AdoptionContainerMissing
	}
	return d.Identity, ""
}

func instanceOwner(tx *gorm.DB, i dto.Instance) (string, error) {
	var rows []instanceRow
	if err := tx.Where("environment_id = ? AND id <> ?", i.EnvironmentID, i.ID).Find(&rows).Error; err != nil {
		return "", err
	}
	for _, row := range rows {
		var other dto.Instance
		if err := json.Unmarshal(row.Data, &other); err != nil {
			return "", err
		}
		if other.Identity.ContainerID == i.Identity.ContainerID {
			return other.ID, nil
		}
	}
	return "", nil
}
func instanceOpen(tx *gorm.DB, id string) (bool, error) {
	var n int64
	if err := tx.Model(&reservation{}).Where("instance_id = ?", id).Count(&n).Error; err != nil {
		return false, err
	}
	if n != 0 {
		return true, nil
	}
	err := tx.Model(&Record{}).Where("instance_id = ? AND finished_at IS NULL", id).Count(&n).Error
	return n != 0, err
}
func casInstance(tx *gorm.DB, original []byte, i dto.Instance) error {
	data, err := marshal(i)
	if err != nil {
		return err
	}
	result := tx.Model(&instanceRow{}).Where("id = ? AND data = ?", i.ID, original).Update("data", data)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return dto.Fail(dto.ContainerChanged, 409)
	}
	return nil
}

// Adopt discovers before locking, then fences identity, registration, ownership
// and unfinished work in the same module-gated transaction as the receipt.
func (m *Module) Adopt(ctx context.Context, instanceID string) (dto.Adoption, error) {
	for attempt := 0; attempt < 2; attempt++ {
		result, err := m.adopt(ctx, instanceID)
		if attempt == 0 && reason(err, "") == dto.RegistrationChanged {
			continue
		}
		return result, err
	}
	panic("unreachable")
}
func (m *Module) adopt(ctx context.Context, id string) (dto.Adoption, error) {
	var original instanceRow
	if err := m.Hub.DB.WithContext(ctx).First(&original, "id = ?", id).Error; err != nil {
		return dto.Adoption{}, err
	}
	var i dto.Instance
	if err := json.Unmarshal(original.Data, &i); err != nil {
		return dto.Adoption{}, err
	}
	answer := dto.Adoption{Outcome: dto.AdoptionRefused, Instance: i}
	reg, err := m.Transport.Registration(ctx, i.EnvironmentID)
	if err != nil {
		return answer, err
	}
	inv, err := m.Transport.Inventory(ctx, i)
	current := err == nil
	var identity dto.Identity
	if current {
		identity = i.Identity
		identity.StartedAt = inv.Identity.StartedAt
	} else {
		if reason(err, "") != dto.ContainerChanged {
			return answer, err
		}
		rows, er := m.Transport.Discover(ctx, i.EnvironmentID)
		if er != nil {
			return answer, er
		}
		identity, answer.Reason = adoptionCandidate(i, rows)
	}
	m.Hub.mu.Lock()
	defer m.Hub.mu.Unlock()
	err = m.Modules.Admit(ctx, features.Proxy, func(ctx context.Context) error {
		return m.Hub.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var row instanceRow
			if er := tx.First(&row, "id = ?", id).Error; er != nil {
				return er
			}
			var stored dto.Instance
			if er := json.Unmarshal(row.Data, &stored); er != nil {
				return er
			}
			answer.Instance = stored
			nowReg, er := registrationIn(tx, i.EnvironmentID)
			if er != nil {
				return er
			}
			if nowReg != reg {
				return dto.Fail(dto.RegistrationChanged, 409)
			}
			if !bytes.Equal(row.Data, original.Data) {
				// A competing adopter already committed this exact identity: no second receipt.
				if stored.Identity == identity && sameSource(stored.Source, i.Source) && stored.Kind == i.Kind && stored.Registration == reg {
					answer.Outcome, answer.Reason = dto.AdoptionCurrent, ""
					return nil
				}
				return dto.Fail(dto.ContainerChanged, 409)
			}
			if answer.Reason != "" {
				return nil
			}
			if !current {
				open, er := instanceOpen(tx, id)
				if er != nil {
					return er
				}
				if open {
					answer.Reason = dto.AdoptionOperationOpen
					return nil
				}
				candidate := i
				candidate.Identity = identity
				owner, er := instanceOwner(tx, candidate)
				if er != nil {
					return er
				}
				if owner != "" {
					answer.Reason, answer.SupersededBy = dto.AdoptionHeldByInstance, owner
					return nil
				}
			}
			i.Identity = identity
			i.Registration = reg
			if current {
				i.Generation, i.Eligibility = inv.Generation, inv.Eligibility
			}
			if er := casInstance(tx, original.Data, i); er != nil {
				return er
			}
			answer.Instance = i
			if current {
				answer.Outcome = dto.AdoptionCurrent
				return nil
			}
			now := time.Now().UTC()
			o := dto.Operation{Kind: dto.AdoptionOperation, ID: uuid.NewString(), InstanceID: id, Generation: i.Generation, Registration: reg, State: dto.Applied, Phase: dto.Phase(dto.Applied), Target: identity.ContainerName, CreatedAt: now, FinishedAt: &now, Adoption: &dto.AdoptionReceipt{PreviousContainerID: stored.Identity.ContainerID, ContainerID: identity.ContainerID, StartedAt: identity.StartedAt}}
			request, er := marshal(dto.ApplyRequest{Kind: dto.AdoptionOperation, ID: o.ID, Instance: i, Generation: i.Generation, Registration: reg})
			if er != nil {
				return er
			}
			result, er := marshal(o)
			if er != nil {
				return er
			}
			if er = tx.Create(&Record{ID: o.ID, InstanceID: id, Generation: i.Generation, Registration: reg, State: o.State, Request: request, Result: result, CreatedAt: now, FinishedAt: &now}).Error; er != nil {
				return er
			}
			answer.Outcome, answer.OperationID = dto.AdoptionAdopted, o.ID
			return nil
		})
	})
	if p, ok := errors.AsType[*moduledto.Problem](err); ok && p.Code == "feature-disabled" {
		answer.Outcome, answer.Reason = dto.AdoptionRefused, dto.AdoptionModuleOff
		return answer, nil
	}
	if err != nil {
		answer.Outcome, answer.OperationID = dto.AdoptionRefused, ""
		answer.Instance, _ = m.Instance(ctx, id)
	}
	return answer, err
}

// Registration reads in a transaction use its connection (SQLite pools may have
// only one connection); the fingerprint matches HostTransport.Registration.
func registrationIn(tx *gorm.DB, id string) (string, error) {
	if id == "0" {
		return "local", nil
	}
	var e environment.Environment
	if err := tx.First(&e, "id = ?", id).Error; err != nil {
		return "", err
	}
	return environmentRegistration(&e), nil
}
func (m *Module) instanceSnapshot(ctx context.Context, id string) (dto.Instance, []byte, error) {
	var row instanceRow
	if err := m.Hub.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return dto.Instance{}, nil, err
	}
	var i dto.Instance
	err := json.Unmarshal(row.Data, &i)
	return i, row.Data, err
}
