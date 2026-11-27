// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original combined update: hub first, then every eligible agent.

package upgrade

import (
	"context"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The fleet driver polls while a plan is active; agents run at most
// fleetParallel at a time, and a busy agent is retried for fleetBusyLimit.
const (
	fleetTick      = 2 * time.Second
	fleetParallel  = 3
	fleetBusyRetry = 15 * time.Second
	fleetBusyLimit = 5 * time.Minute
)

// SetUpdatePermission supplies the check that the starting administrator may
// still update before each new effect. Without it no new effect is started.
func (s *Service) SetUpdatePermission(check func(context.Context, string) (bool, error)) {
	s.allowed = check
}
func (s *Service) fleet() core.FleetStore { return core.FleetStore{DB: s.db.DB} }
func (s *Service) checkHub(ctx context.Context) (dto.SelfUpdateCheck, error) {
	if s.localCheck != nil {
		return s.localCheck(ctx)
	}
	return s.Check(ctx, "0")
}
func (s *Service) wakeFleet() {
	select {
	case s.fleetWake <- struct{}{}:
	default:
	}
}

// StartFleet admits one combined update for the version the administrator
// confirmed. The plan and its parent activity are written in one transaction.
func (s *Service) StartFleet(ctx context.Context, request dto.FleetUpdateRequest, userID, username string) (dto.FleetUpdateResponse, error) {
	if s.role() != "hub" {
		return dto.FleetUpdateResponse{}, core.Refuse("update.invalid-target")
	}
	check, err := s.checkHub(ctx)
	if err != nil {
		return dto.FleetUpdateResponse{}, err
	}
	plan := core.FleetPlan{SchemaVersion: core.FleetSchemaVersion, FleetID: uuid.NewString(), RunID: uuid.NewString(), Phase: "hub", StartedBy: userID, StartedByName: username, CreatedAt: time.Now().UTC()}
	plan.Hub = core.FleetTarget{EnvironmentID: "0", OpID: core.FleetOpID(plan.FleetID, "0"), State: core.TargetPending}
	switch check.Eligibility.State {
	case "available":
		plan.TargetVersion = check.Eligibility.Candidate.Version
	case "current":
		plan.TargetVersion = s.runningVersion()
		plan.Hub.State, plan.Hub.Outcome, plan.Hub.Code = core.TargetSkipped, "current", dto.FleetCurrent
		plan.Phase = "agents"
	default:
		code := check.Eligibility.Code
		if code == "" {
			code = "update.invalid-target"
		}
		return dto.FleetUpdateResponse{}, core.Refuse(code)
	}
	if request.TargetVersion != plan.TargetVersion {
		return dto.FleetUpdateResponse{}, core.Refuse("update.invalid-target")
	}
	err = s.fleet().Admit(ctx, plan, func(tx *gorm.DB, plan *core.FleetPlan) error {
		var hosts []environment.Environment
		if err := tx.Where("id <> ?", "0").Order("created_at, id").Find(&hosts).Error; err != nil {
			return err
		}
		for _, host := range hosts {
			plan.Agents = append(plan.Agents, core.FleetTarget{EnvironmentID: host.ID, CredentialRevision: host.CredentialRevision, RegisteredAt: host.CreatedAt, OpID: core.FleetOpID(plan.FleetID, host.ID), State: core.TargetPending})
		}
		now := time.Now().UTC()
		parent := activity.Activity{BaseModel: database.BaseModel{ID: plan.RunID, CreatedAt: now}, EnvironmentID: "0", Type: activityDTO.TypeFleetUpdate, Status: activityDTO.StatusRunning, StartedAt: now, StartedByUserID: new(userID), StartedByUsername: new(username), MessageCode: string(activityDTO.CodeRunStarted), ResourceType: new("system"), ResourceID: new("0"), Metadata: database.JSON{"fleetId": plan.FleetID, "targetVersion": plan.TargetVersion}}
		return tx.Create(&parent).Error
	})
	if err != nil {
		return dto.FleetUpdateResponse{}, err
	}
	s.wakeFleet()
	out := dto.FleetUpdateResponse{FleetID: plan.FleetID, RunID: plan.RunID, HubPhase: "current"}
	if plan.Phase == "hub" {
		out.HubPhase, out.HubOpID = "update", plan.Hub.OpID
	}
	return out, nil
}

// FleetStatus returns the active or last combined update, read-only.
func (s *Service) FleetStatus(ctx context.Context) (dto.FleetUpdateStatus, error) {
	plan, _, err := s.fleet().Read(ctx)
	if err != nil && !errors.Is(err, core.ErrFleetUnreadable) {
		return dto.FleetUpdateStatus{}, err
	}
	out := dto.FleetUpdateStatus{Agents: []dto.FleetUpdateTarget{}}
	if plan == nil {
		return out, nil
	}
	view := func(t core.FleetTarget) dto.FleetUpdateTarget {
		return dto.FleetUpdateTarget{EnvironmentID: t.EnvironmentID, State: t.State, Outcome: t.Outcome, Code: t.Code, RunID: t.RunID, OpID: t.OpID}
	}
	hub := view(plan.Hub)
	out = dto.FleetUpdateStatus{Active: plan.Phase != "settled", FleetID: plan.FleetID, RunID: plan.RunID, Phase: plan.Phase, Outcome: plan.Outcome, TargetVersion: plan.TargetVersion, Hub: &hub, Agents: []dto.FleetUpdateTarget{}}
	for _, agent := range plan.Agents {
		out.Agents = append(out.Agents, view(agent))
	}
	return out, nil
}

// driveFleet runs for the application lifetime; it advances an active plan
// every tick and sleeps until admission wakes it otherwise.
func (s *Service) driveFleet(ctx context.Context) {
	for ctx.Err() == nil {
		done, _ := s.advanceFleet(ctx)
		var tick <-chan time.Time
		var timer *time.Timer
		if !done {
			timer = time.NewTimer(fleetTick)
			tick = timer.C
		}
		select {
		case <-ctx.Done():
		case <-s.fleetWake:
		case <-tick:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

// advanceFleet reports done when no plan needs further work.
func (s *Service) advanceFleet(ctx context.Context) (bool, error) {
	plan, raw, err := s.fleet().Read(ctx)
	if errors.Is(err, core.ErrFleetUnreadable) {
		return true, s.fleet().Release(ctx, raw, func(tx *gorm.DB, runID string) error {
			return settleParentTx(tx, runID, activityDTO.StatusFailed, "activity.failed", database.JSON{"reason": dto.FleetUnreadable})
		})
	}
	if err != nil {
		return false, err
	}
	if plan == nil || plan.Phase == "settled" {
		return true, nil
	}
	if plan.Phase == "hub" {
		return false, s.advanceHub(ctx, *plan)
	}
	return false, s.advanceAgents(ctx, *plan)
}

func (s *Service) mayUpdate(ctx context.Context, plan core.FleetPlan) bool {
	if s.allowed == nil {
		return false
	}
	ok, err := s.allowed(ctx, plan.StartedBy)
	return err == nil && ok
}

func (s *Service) advanceHub(ctx context.Context, plan core.FleetPlan) error {
	op, err := s.store.Read(ctx)
	if err != nil {
		return err
	}
	if plan.Hub.State == core.TargetPending {
		if op != nil && op.OpID == plan.Hub.OpID {
			return s.changeTarget(ctx, plan, "0", func(t *core.FleetTarget) { t.State, t.RunID = core.TargetReserved, op.RunID })
		}
		if !s.mayUpdate(ctx, plan) {
			return s.failHub(ctx, plan, "skipped", dto.FleetPermission)
		}
		if plan.Hub.TargetRef == "" {
			check, err := s.checkHub(ctx)
			if err != nil {
				return err
			}
			if check.Eligibility.State != "available" || check.Eligibility.Candidate == nil || check.Eligibility.Candidate.Version != plan.TargetVersion {
				return s.failHub(ctx, plan, "skipped", firstCode(check.Eligibility.Code, "update.invalid-target"))
			}
			ref, err := s.resolve(ctx, repository("hub"), plan.TargetVersion, s.transport)
			if err != nil {
				return s.failHub(ctx, plan, "skipped", "update.check-unavailable")
			}
			return s.changeTarget(ctx, plan, "0", func(t *core.FleetTarget) { t.TargetRef = ref })
		}
		envelope := dto.SelfUpdateEnvelope{OpID: plan.Hub.OpID, TargetVersion: plan.TargetVersion, TargetReference: plan.Hub.TargetRef, HubVersion: s.runningVersion()}
		reserve := s.reserveLocal
		if s.reserveHub != nil {
			reserve = s.reserveHub
		}
		reserved, err := reserve(ctx, envelope, plan.StartedBy, plan.StartedByName, &fleetLink{parentRunID: plan.RunID})
		var problem *core.Problem
		if errors.As(err, &problem) {
			return s.failHub(ctx, plan, "skipped", problem.Code)
		}
		if err != nil {
			if time.Since(plan.CreatedAt) > core.OperationTimeout {
				code := "update.check-unavailable"
				if hostBusy(err) {
					code = "host.busy"
				}
				return s.failHub(ctx, plan, "skipped", code)
			}
			return err
		}
		return s.changeTarget(ctx, plan, "0", func(t *core.FleetTarget) { t.State, t.RunID = core.TargetReserved, reserved.RunID })
	}
	// Agents follow only the planned operation's healthy settlement on the target version.
	if op == nil || op.OpID != plan.Hub.OpID {
		return s.failHub(ctx, plan, "unknown", core.CodeUnknown)
	}
	if op.Phase != "settled" {
		return nil
	}
	if op.Outcome == "healthy" && s.runningVersion() == plan.TargetVersion {
		_, err := s.fleet().Change(ctx, plan.FleetID, func(_ *gorm.DB, p *core.FleetPlan) error {
			p.Hub.State, p.Hub.Outcome, p.Hub.Code = core.TargetSettled, "healthy", ""
			p.Phase = "agents"
			return nil
		})
		return err
	}
	outcome := op.Outcome
	if outcome == "" || outcome == "healthy" {
		outcome = "unknown"
	}
	return s.failHub(ctx, plan, outcome, firstCode(op.Code, core.CodeFailed))
}

// failHub ends the plan: no agent is touched, each gets a terminal skip child.
func (s *Service) failHub(ctx context.Context, plan core.FleetPlan, outcome, code string) error {
	_, err := s.fleet().Change(ctx, plan.FleetID, func(tx *gorm.DB, p *core.FleetPlan) error {
		p.Hub.State, p.Hub.Outcome, p.Hub.Code = core.TargetSettled, outcome, code
		if outcome == "skipped" {
			p.Hub.State = core.TargetSkipped
		}
		for i := range p.Agents {
			if !p.Agents[i].Terminal() {
				if err := skipTx(tx, p, &p.Agents[i], dto.FleetHubFailed); err != nil {
					return err
				}
			}
		}
		return settlePlanTx(tx, p)
	})
	return err
}

func (s *Service) changeTarget(ctx context.Context, plan core.FleetPlan, id string, change func(*core.FleetTarget)) error {
	_, err := s.fleet().Change(ctx, plan.FleetID, func(_ *gorm.DB, p *core.FleetPlan) error {
		if target := p.Target(id); target != nil {
			change(target)
		}
		return nil
	})
	return err
}

// skipTx records a host the plan did not update as a terminal child run.
func skipTx(tx *gorm.DB, plan *core.FleetPlan, target *core.FleetTarget, code string) error {
	now := time.Now().UTC()
	// Message codes stay in the shared activity vocabulary; the reason is metadata.
	status, outcome, message := activityDTO.StatusCancelled, "skipped", "activity.cancelled"
	if code == dto.FleetCurrent {
		status, outcome, message = activityDTO.StatusSuccess, "current", "activity.success"
	}
	run := activity.Activity{BaseModel: database.BaseModel{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("docklet.fleet-skip:"+target.OpID)).String(), CreatedAt: now}, EnvironmentID: target.EnvironmentID, Type: activityDTO.TypeAgentUpdate, Status: status, StartedAt: now, EndedAt: &now, StartedByUserID: new(plan.StartedBy), StartedByUsername: new(plan.StartedByName), MessageCode: message, ResourceType: new("system"), ResourceID: new(target.EnvironmentID), BatchID: new(plan.RunID), Metadata: database.JSON{"fleetId": plan.FleetID, "targetVersion": plan.TargetVersion, "skipped": true, "reason": code}}
	if err := tx.Create(&run).Error; err != nil {
		return err
	}
	target.State, target.Outcome, target.Code, target.RunID = core.TargetSkipped, outcome, code, run.ID
	return nil
}

// settlePlanTx applies the result table of the design note to the parent run.
func settlePlanTx(tx *gorm.DB, plan *core.FleetPlan) error {
	updated, current, failed, skipped := 0, 0, 0, 0
	hubUpdated := plan.Hub.Outcome == "healthy"
	for _, agent := range plan.Agents {
		switch {
		case agent.Outcome == "healthy":
			updated++
		case agent.Outcome == "current":
			current++
		case agent.State == core.TargetSkipped:
			skipped++
		default:
			failed++
		}
	}
	outcome := "success"
	switch {
	case plan.Hub.Outcome != "healthy" && plan.Hub.Outcome != "current":
		outcome = "failed"
	case failed == 0 && skipped == 0:
	case hubUpdated || updated > 0 || failed == 0:
		outcome = "partial"
	default:
		outcome = "failed"
	}
	plan.Phase, plan.Outcome = "settled", outcome
	status, code := activityDTO.StatusSuccess, "activity.success"
	if outcome != "success" {
		status, code = activityDTO.StatusFailed, "activity.failed"
	}
	metadata := database.JSON{"fleetId": plan.FleetID, "targetVersion": plan.TargetVersion, "updated": updated, "current": current, "skipped": skipped, "failed": failed, "hubUpdated": hubUpdated}
	if plan.Hub.Outcome != "healthy" && plan.Hub.Outcome != "current" {
		metadata["reason"] = dto.FleetHubFailed
	}
	if outcome == "partial" {
		metadata["jobStatus"] = "partial"
	}
	return settleParentTx(tx, plan.RunID, status, code, metadata)
}

func settleParentTx(tx *gorm.DB, runID string, status activityDTO.Status, code string, metadata database.JSON) error {
	now := time.Now().UTC()
	values := map[string]any{"status": status, "message_code": code, "ended_at": now, "updated_at": now}
	if len(metadata) > 0 {
		values["metadata"] = metadata
	}
	return tx.Model(&activity.Activity{}).Where("id=? AND status IN ?", runID, []activityDTO.Status{activityDTO.StatusRunning, activityDTO.StatusQueued}).Updates(values).Error
}

func firstCode(code, fallback string) string {
	if code == "" {
		return fallback
	}
	return code
}
