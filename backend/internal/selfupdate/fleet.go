// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original durable plan for the combined hub-then-agents update.

package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FleetKey holds the single combined update plan. Its runId/phase fields share
// the shape that activity repair reads for every self-update owner.
const FleetKey = "selfupdate.fleet"
const FleetSchemaVersion = 1

// Fleet target states. A target is terminal once skipped or settled.
const (
	TargetPending  = "pending"
	TargetReserved = "reserved"
	TargetSkipped  = "skipped"
	TargetSettled  = "settled"
)

// FleetTarget is one host of the plan. Identity fields are frozen at admission;
// State, Outcome, Code, RunID, TargetRef and BusySince advance by CAS.
type FleetTarget struct {
	EnvironmentID      string     `json:"environmentId"`
	CredentialRevision int64      `json:"credentialRevision,omitempty"`
	RegisteredAt       time.Time  `json:"registeredAt,omitzero"`
	OpID               string     `json:"opId"`
	State              string     `json:"state"`
	Outcome            string     `json:"outcome,omitempty"`
	Code               string     `json:"code,omitempty"`
	RunID              string     `json:"runId,omitempty"`
	TargetRef          string     `json:"targetRef,omitempty"`
	BusySince          *time.Time `json:"busySince,omitempty"`
	NextTry            *time.Time `json:"nextTry,omitempty"`
}

func (t FleetTarget) Terminal() bool { return t.State == TargetSkipped || t.State == TargetSettled }

type FleetPlan struct {
	SchemaVersion int           `json:"schemaVersion"`
	FleetID       string        `json:"fleetId"`
	RunID         string        `json:"runId"`
	Phase         string        `json:"phase"`
	Outcome       string        `json:"outcome,omitempty"`
	TargetVersion string        `json:"targetVersion"`
	StartedBy     string        `json:"startedBy"`
	StartedByName string        `json:"startedByName"`
	CreatedAt     time.Time     `json:"createdAt"`
	Hub           FleetTarget   `json:"hub"`
	Agents        []FleetTarget `json:"agents"`
}

// Target returns the hub ("0") or agent target with the environment id.
func (p *FleetPlan) Target(id string) *FleetTarget {
	if id == "0" {
		return &p.Hub
	}
	for i := range p.Agents {
		if p.Agents[i].EnvironmentID == id {
			return &p.Agents[i]
		}
	}
	return nil
}

// FleetOpID derives the operation id of one host, so a resumed driver joins
// the reserved operation instead of creating a second one.
func FleetOpID(fleetID, environmentID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("docklet.fleet:"+fleetID+":"+environmentID)).String()
}

// ErrFleetUnreadable marks a stored plan this version cannot interpret.
var ErrFleetUnreadable = errors.New("update.fleet-unreadable")

// DecodeFleet fails closed on unknown versions, missing identity fields and
// state combinations the driver cannot advance, before any effect.
func DecodeFleet(raw string) (FleetPlan, error) {
	var plan FleetPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return plan, ErrFleetUnreadable
	}
	if plan.SchemaVersion != FleetSchemaVersion || plan.FleetID == "" || plan.RunID == "" || plan.TargetVersion == "" || plan.StartedBy == "" || plan.CreatedAt.IsZero() {
		return plan, ErrFleetUnreadable
	}
	if plan.Hub.EnvironmentID != "0" || plan.Hub.OpID != FleetOpID(plan.FleetID, "0") || !validTargetState(plan.Hub.State) {
		return plan, ErrFleetUnreadable
	}
	seen := map[string]bool{}
	for _, agent := range plan.Agents {
		if agent.EnvironmentID == "" || agent.EnvironmentID == "0" || seen[agent.EnvironmentID] || agent.OpID != FleetOpID(plan.FleetID, agent.EnvironmentID) || !validTargetState(agent.State) || agent.RegisteredAt.IsZero() {
			return plan, ErrFleetUnreadable
		}
		seen[agent.EnvironmentID] = true
		// No agent leaves pending before the hub settled.
		if plan.Phase == "hub" && agent.State != TargetPending && agent.State != TargetSkipped {
			return plan, ErrFleetUnreadable
		}
	}
	switch plan.Phase {
	case "hub":
		if plan.Hub.Terminal() {
			return plan, ErrFleetUnreadable
		}
	case "agents":
		if !plan.Hub.Terminal() || (plan.Hub.Outcome != "healthy" && plan.Hub.Outcome != "current") {
			return plan, ErrFleetUnreadable
		}
	case "settled":
		if !plan.Hub.Terminal() || (plan.Outcome != "success" && plan.Outcome != "partial" && plan.Outcome != "failed") {
			return plan, ErrFleetUnreadable
		}
		for _, agent := range plan.Agents {
			if !agent.Terminal() {
				return plan, ErrFleetUnreadable
			}
		}
	default:
		return plan, ErrFleetUnreadable
	}
	return plan, nil
}

func validTargetState(state string) bool {
	switch state {
	case TargetPending, TargetReserved, TargetSkipped, TargetSettled:
		return true
	}
	return false
}

type FleetStore struct{ DB *gorm.DB }

// Read returns the stored plan and its raw value. An unreadable plan returns
// ErrFleetUnreadable together with the raw value, so the caller can release it.
func (s FleetStore) Read(ctx context.Context) (*FleetPlan, string, error) {
	var entry kv.KVEntry
	err := s.DB.WithContext(ctx).First(&entry, "key=?", FleetKey).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	plan, err := DecodeFleet(entry.Value)
	if err != nil {
		var owner struct {
			Released bool `json:"released"`
		}
		// Only the marker written by Release stands for "no plan".
		if json.Unmarshal([]byte(entry.Value), &owner) == nil && owner.Released {
			return nil, entry.Value, nil
		}
		return nil, entry.Value, err
	}
	return &plan, entry.Value, nil
}

// Admit writes a new plan when the slot is empty or settled; create runs in
// the same transaction and receives the plan before it is stored.
func (s FleetStore) Admit(ctx context.Context, plan FleetPlan, create func(*gorm.DB, *FleetPlan) error) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entry kv.KVEntry
		err := tx.First(&entry, "key=?", FleetKey).Error
		if err == nil {
			// Only a validated settled plan or a release marker may be replaced; an
			// unreadable value is released by the driver first, failing its parent.
			if !replaceable(entry.Value) {
				return Refuse("update.all-busy")
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if create != nil {
			if err := create(tx, &plan); err != nil {
				return err
			}
		}
		raw, err := json.Marshal(plan)
		if err != nil {
			return err
		}
		entry = kv.KVEntry{Key: FleetKey, Value: string(raw)}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"})}).Create(&entry).Error
	})
}

// Change applies change to the plan with fleetID under an exact-value CAS.
// It reports false without writing when the slot holds another or a settled plan.
func (s FleetStore) Change(ctx context.Context, fleetID string, change func(*gorm.DB, *FleetPlan) error) (bool, error) {
	changed := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entry kv.KVEntry
		if err := tx.First(&entry, "key=?", FleetKey).Error; err != nil {
			return err
		}
		plan, err := DecodeFleet(entry.Value)
		if err != nil {
			return err
		}
		if plan.FleetID != fleetID || plan.Phase == "settled" {
			return nil
		}
		if err := change(tx, &plan); err != nil {
			return err
		}
		raw, err := json.Marshal(plan)
		if err != nil {
			return err
		}
		update := tx.Model(&kv.KVEntry{}).Where("key=? AND value=?", FleetKey, entry.Value).Updates(map[string]any{"value": string(raw), "updated_at": time.Now()})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("fleet plan changed concurrently")
		}
		changed = true
		return nil
	})
	return changed, err
}

// Release replaces an unreadable plan with a settled marker keeping its run id
// when it can be read, so admission is possible again and repair stops protecting it.
func (s FleetStore) Release(ctx context.Context, raw string, settle func(*gorm.DB, string) error) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner struct {
			RunID string `json:"runId"`
		}
		_ = json.Unmarshal([]byte(raw), &owner)
		if settle != nil && owner.RunID != "" {
			if err := settle(tx, owner.RunID); err != nil {
				return err
			}
		}
		marker, _ := json.Marshal(map[string]any{"released": true, "phase": "settled", "runId": owner.RunID, "code": ErrFleetUnreadable.Error()})
		update := tx.Model(&kv.KVEntry{}).Where("key=? AND value=?", FleetKey, raw).Updates(map[string]any{"value": string(marker), "updated_at": time.Now()})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("fleet plan changed concurrently")
		}
		return nil
	})
}

func replaceable(raw string) bool {
	if plan, err := DecodeFleet(raw); err == nil {
		return plan.Phase == "settled"
	}
	var marker struct {
		Released bool `json:"released"`
	}
	return json.Unmarshal([]byte(raw), &marker) == nil && marker.Released
}

// ErrNotReserved reports that a dispatch marker was not acquired because the
// operation already left the reserved phase.
var ErrNotReserved = errors.New("self-update operation is not reserved")

// AcquireDispatch moves the operation from reserved to helper-started. Only the
// caller that receives true may send the one dispatch request.
func (s *Store) AcquireDispatch(ctx context.Context, id string, check func(*gorm.DB, *Operation) error) (bool, error) {
	changed, err := s.Change(ctx, id, func(tx *gorm.DB, current *Operation) error {
		if current.Phase != "reserved" {
			return ErrNotReserved
		}
		if check != nil {
			if err := check(tx, current); err != nil {
				return err
			}
		}
		current.Phase = "helper-started"
		return nil
	})
	if errors.Is(err, ErrNotReserved) {
		return false, nil
	}
	return changed, err
}
