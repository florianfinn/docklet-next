// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original agent phase of the combined update.

package upgrade

import (
	"context"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	envDTO "github.com/florianfinn/docklet-next/types/environment"
	dto "github.com/florianfinn/docklet-next/types/system"
	product "github.com/florianfinn/docklet-next/types/version"
	"gorm.io/gorm"
)

// agentStep is the decision for one pending agent in one driver round.
type agentStep struct {
	skip    string // terminal skip code
	busy    bool
	ref     string // resolved target reference to store before reserving
	join    *core.Operation
	created *core.Operation
	env     environment.Environment
}

// advanceAgents settles observed operations, starts pending agents within the
// parallel limit and settles the plan once every target is terminal.
func (s *Service) advanceAgents(ctx context.Context, plan core.FleetPlan) error {
	inflight := 0
	for _, agent := range plan.Agents {
		if agent.State != core.TargetReserved {
			continue
		}
		store := s.remoteStore(agent.EnvironmentID)
		op, err := store.Read(ctx)
		if err != nil {
			return err
		}
		switch {
		case op == nil || op.OpID != agent.OpID:
			err = s.changeTarget(ctx, plan, agent.EnvironmentID, func(t *core.FleetTarget) {
				t.State, t.Outcome, t.Code = core.TargetSettled, "unknown", core.CodeUnknown
			})
		case op.Phase == "settled":
			err = s.changeTarget(ctx, plan, agent.EnvironmentID, func(t *core.FleetTarget) {
				t.State, t.Outcome, t.Code = core.TargetSettled, op.Outcome, op.Code
			})
		default:
			inflight++
			s.resumeRemote(store.Key)
		}
		if err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	for _, agent := range plan.Agents {
		if agent.State != core.TargetPending || inflight >= fleetParallel || (agent.NextTry != nil && now.Before(*agent.NextTry)) {
			continue
		}
		step, err := s.agentStep(ctx, plan, agent)
		if err != nil {
			return err
		}
		if step.created != nil || step.join != nil {
			inflight++
		}
		if err := s.applyAgentStep(ctx, plan, agent, step, now); err != nil {
			return err
		}
		if step.created != nil {
			op, env := *step.created, step.env
			store := s.remoteStore(agent.EnvironmentID)
			s.launch(store.Key, func(ctx context.Context) { s.dispatchRemote(ctx, store, op, env) })
		}
	}
	current, _, err := s.fleet().Read(ctx)
	if err != nil || current == nil || current.FleetID != plan.FleetID {
		return err
	}
	for _, agent := range current.Agents {
		if !agent.Terminal() {
			return nil
		}
	}
	_, err = s.fleet().Change(ctx, plan.FleetID, func(tx *gorm.DB, p *core.FleetPlan) error { return settlePlanTx(tx, p) })
	return err
}

func (s *Service) applyAgentStep(ctx context.Context, plan core.FleetPlan, agent core.FleetTarget, step agentStep, now time.Time) error {
	_, err := s.fleet().Change(ctx, plan.FleetID, func(tx *gorm.DB, p *core.FleetPlan) error {
		target := p.Target(agent.EnvironmentID)
		if target == nil || target.State != core.TargetPending {
			return nil
		}
		switch {
		case step.join != nil:
			target.State, target.RunID = core.TargetReserved, step.join.RunID
		case step.created != nil:
			target.State, target.RunID = core.TargetReserved, step.created.RunID
		case step.ref != "":
			target.TargetRef = step.ref
		case step.busy:
			if target.BusySince == nil {
				target.BusySince = &now
			}
			if now.Sub(*target.BusySince) >= fleetBusyLimit {
				return skipTx(tx, p, target, "host.busy")
			}
			next := now.Add(fleetBusyRetry)
			target.NextTry = &next
		case step.skip != "":
			return skipTx(tx, p, target, step.skip)
		}
		return nil
	})
	return err
}

// agentStep decides one pending agent. Only definitive refusals skip; an
// unexpected local error is returned and retried in the next round.
func (s *Service) agentStep(ctx context.Context, plan core.FleetPlan, agent core.FleetTarget) (agentStep, error) {
	store := s.remoteStore(agent.EnvironmentID)
	existing, err := store.Read(ctx)
	if err != nil {
		return agentStep{}, err
	}
	// A reserved planned operation is joined, never reserved or sent again.
	if existing != nil && existing.OpID == agent.OpID {
		return agentStep{join: existing}, nil
	}
	env, err := s.remoteEnvironment(ctx, agent.EnvironmentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agentStep{skip: dto.FleetTargetChanged}, nil
		}
		return agentStep{skip: "agent.unavailable"}, nil
	}
	if env.CredentialRevision != agent.CredentialRevision || !env.CreatedAt.Equal(agent.RegisteredAt) {
		return agentStep{skip: dto.FleetTargetChanged}, nil
	}
	// Recovery routes answer below the minimum contract, so the contract is read explicitly.
	if _, err := s.environments.CheckAgentCompatibility(ctx, env.ID); errors.Is(err, remenv.ErrAgentUpdateRequired) {
		return agentStep{skip: dto.FleetAgentTooOld}, nil
	} else if err != nil {
		return agentStep{skip: "agent.unavailable"}, nil
	}
	check, err := s.readRemote(ctx, env)
	switch {
	case errors.Is(err, remenv.ErrAgentUpdateRequired):
		return agentStep{skip: dto.FleetAgentTooOld}, nil
	case core.HasCode(err, core.CodeUnknown):
		return agentStep{skip: dto.FleetTargetChanged}, nil
	case err != nil:
		return agentStep{skip: "agent.unavailable"}, nil
	}
	running, okRunning := product.ParseProduct(check.Version)
	target, okTarget := product.ParseProduct(plan.TargetVersion)
	if !okRunning || !okTarget {
		return agentStep{skip: "update.system-version-unknown"}, nil
	}
	if c := running.Compare(target); c == 0 {
		return agentStep{skip: dto.FleetCurrent}, nil
	} else if c > 0 {
		return agentStep{skip: dto.FleetNewerThanHub}, nil
	}
	switch check.Eligibility.State {
	case "not-eligible":
		if check.Eligibility.Code == "host.busy" {
			return agentStep{busy: true}, nil
		}
		return agentStep{skip: firstCode(check.Eligibility.Code, "update.invalid-target")}, nil
	case "unavailable":
		return agentStep{skip: firstCode(check.Eligibility.Code, "agent.unavailable")}, nil
	}
	if agent.TargetRef == "" {
		ref, err := s.resolve(ctx, repository("agent"), plan.TargetVersion, s.transport)
		if err != nil {
			return agentStep{skip: "update.check-unavailable"}, nil
		}
		return agentStep{ref: ref}, nil
	}
	// Authorization is checked at the effect boundary, after the preparatory reads.
	if !s.mayUpdate(ctx, plan) {
		return agentStep{skip: dto.FleetPermission}, nil
	}
	op := core.Operation{OpID: agent.OpID, Role: "agent", EnvironmentID: agent.EnvironmentID, CredentialRevision: agent.CredentialRevision, TargetRef: agent.TargetRef, TargetVersion: plan.TargetVersion, FromVersion: check.Version, Deadline: time.Now().UTC().Add(core.OperationTimeout)}
	reserved, created, err := s.reserveRun(ctx, store, op, plan.StartedBy, plan.StartedByName, &fleetLink{parentRunID: plan.RunID, registeredAt: agent.RegisteredAt})
	var problem *core.Problem
	switch {
	case core.HasCode(err, "host.busy") || hostBusy(err):
		return agentStep{busy: true}, nil
	case core.HasCode(err, core.CodeUnknown):
		return agentStep{skip: dto.FleetTargetChanged}, nil
	case errors.As(err, &problem):
		return agentStep{skip: problem.Code}, nil
	case err != nil:
		return agentStep{}, err
	case !created:
		return agentStep{join: &reserved}, nil
	}
	return agentStep{created: &reserved, env: env}, nil
}

// hostBusy reports the transactional host admission refusal.
func hostBusy(err error) bool {
	var problem *envDTO.HostProblem
	return errors.As(err, &problem) && problem.Code == "host.busy"
}

// hostIdentity is one registration a version reading belongs to.
type hostIdentity struct {
	id         string
	revision   int64
	registered time.Time
}

func (h hostIdentity) matches(env environment.Environment) bool {
	return env.ID == h.id && env.CredentialRevision == h.revision && env.CreatedAt.Equal(h.registered)
}

// agentsBelow returns the enabled, compatible agents that run a version below
// target; hosts are read in parallel with the bounded remote checks.
func (s *Service) agentsBelow(ctx context.Context, target string) []hostIdentity {
	limit, ok := product.ParseProduct(target)
	if !ok || s.environments == nil {
		return nil
	}
	var hosts []environment.Environment
	if s.db.WithContext(ctx).Where("id <> ? AND enabled = ?", "0", true).Find(&hosts).Error != nil {
		return nil
	}
	found := make(chan *hostIdentity, len(hosts))
	for _, host := range hosts {
		go func() {
			if _, err := s.environments.CheckAgentCompatibility(ctx, host.ID); err != nil {
				found <- nil
				return
			}
			check, err := s.readRemote(ctx, host)
			running, valid := product.ParseProduct(check.Version)
			if err != nil || !valid || running.Compare(limit) >= 0 {
				found <- nil
				return
			}
			found <- &hostIdentity{id: host.ID, revision: host.CredentialRevision, registered: host.CreatedAt}
		}()
	}
	var below []hostIdentity
	for range hosts {
		if identity := <-found; identity != nil {
			below = append(below, *identity)
		}
	}
	return below
}
