// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original trusted hub observations and per-target window dispatch.

package updater

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"

	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	st "github.com/florianfinn/docklet-next/types/scheduler"
)

func (s *UpdaterService) RunUpdateCheck(ctx context.Context) (st.Outcome, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return st.Outcome{Status: st.Failed, Code: "update.schedule-hub-owned"}, nil
	}
	hosts, err := s.ImagePolicyHosts(ctx)
	if err != nil {
		return st.Outcome{}, err
	}
	out := st.Outcome{Status: st.Succeeded}
	for _, host := range hosts {
		if ctx.Err() != nil {
			return policyResult(out), ctx.Err()
		}
		one, err := s.runUpdateCheckHost(ctx, p, host)
		out.Targets = append(out.Targets, one.Targets...)
		if err != nil {
			return policyResult(out), err
		}
	}
	if err := p.store.PruneHistory(ctx, time.Now().UTC()); err != nil {
		return policyResult(out), err
	}
	return policyResult(out), nil
}

func (s *UpdaterService) runUpdateCheckHost(ctx context.Context, p *imagePolicyService, env string) (st.Outcome, error) {
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return s.recordHostCheckFailure(ctx, p, env)
	}
	targets, err := p.ports.targets(ctx, env)
	if err != nil {
		return s.recordHostCheckFailure(ctx, p, env)
	}
	out := st.Outcome{Status: st.Succeeded}
	present := map[string]bool{}
	for _, target := range targets {
		if !target.Eligible {
			continue
		}
		present[target.Kind+"\x00"+target.ID] = true
		one, err := p.store.Policy(ctx, env, reg, target.ID, target.Kind)
		if err != nil {
			return out, err
		}
		if one.Revision == 0 {
			if target.Kind == "container" {
				one.TargetIdentity = target.Name
			} else if baseline, e := p.ports.baseline(ctx, env, target.ID); e == nil {
				one.TargetIdentity = baseline.TargetIdentity
			}
			one, err = p.store.EnsureObservedPolicy(ctx, one)
			if err != nil {
				return out, err
			}
		}
		var result st.TargetOutcome
		if target.Kind == "container" {
			check, e := p.ports.checkContainer(ctx, env, target)
			if e == nil && check != nil && check.Status != "failed" && check.Status != "unknown" && check.Status != "error" && check.Status != "not_pulled" {
				_, e = s.observeImagePolicy(ctx, p, one, check)
			} else if e == nil {
				e = errors.New("unavailable observation")
			}
			result = policyTarget(target.ID, st.Succeeded, "")
			if e != nil {
				result = policyTarget(target.ID, st.Failed, policyCode(e, "update.check-unavailable"))
			}
			result = s.recordImagePolicyStatus(ctx, p, one, "update-check", result)
		} else {
			result, _ = s.runImagePolicyTarget(ctx, p, one, "", "update-check")
			if result.Status == st.Skipped {
				result.Status = st.Failed
			}
		}
		result = qualifyUpdateTarget(env, target.Kind, target.ID, result)
		out.Targets = append(out.Targets, result)
		if err := jobcontext.Progress(ctx, result); err != nil {
			return out, err
		}
	}
	if err := p.store.PruneAbsent(ctx, env, reg, present, time.Now().UTC()); err != nil {
		return out, err
	}
	return policyResult(out), nil
}

func (s *UpdaterService) recordHostCheckFailure(ctx context.Context, p *imagePolicyService, env string) (st.Outcome, error) {
	var rows []policy.Policy
	if err := p.store.DB.WithContext(ctx).Where("environment_id = ?", env).Find(&rows).Error; err != nil {
		return st.Outcome{}, err
	}
	out := st.Outcome{Status: st.Partial}
	for _, row := range rows {
		result := s.recordImagePolicyStatus(ctx, p, row, "update-check", st.TargetOutcome{Status: st.Failed, Code: "agent.unavailable"})
		out.Targets = append(out.Targets, qualifyUpdateTarget(env, row.TargetKind, row.TargetID, result))
	}
	if len(out.Targets) == 0 {
		out.Targets = append(out.Targets, st.TargetOutcome{ID: env + "~environment~" + env, EnvironmentID: env, TargetID: env, ResourceType: "environment", Status: st.Failed, Code: "agent.unavailable"})
	}
	for _, result := range out.Targets {
		if err := jobcontext.Progress(ctx, result); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *UpdaterService) RunUpdateWindow(ctx context.Context, id string) (st.Outcome, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return st.Outcome{}, err
	}
	previous, found := jobcontext.Run(ctx)
	if !found || previous.ID == "" {
		return st.Outcome{Status: st.Failed, Code: "update.invalid-target"}, nil
	}
	var row policy.Policy
	if err := p.store.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return st.Outcome{Status: st.Skipped, Code: "job.skipped"}, nil
		}
		return st.Outcome{}, err
	}
	if previous.Trigger == "recovery" || previous.AttemptCount > 1 || !p.startedAt.IsZero() && previous.CreatedAt.Before(p.startedAt) {
		return s.ReconcileImagePolicyHost(ctx, row.EnvironmentID, previous)
	}
	if row.Mode != "scheduled" {
		return st.Outcome{Status: st.Skipped, Code: "job.skipped"}, nil
	}
	var host struct{ Enabled bool }
	if p.store.DB.WithContext(ctx).Table("environments").Select("enabled").Where("id = ?", row.EnvironmentID).Take(&host).Error != nil || !host.Enabled {
		return st.Outcome{Status: st.Skipped, Code: "agent.unavailable"}, nil
	}
	blocked, err := s.imagePolicyHostHold(ctx, p, row.EnvironmentID, row.RegistrationID)
	if err != nil {
		return st.Outcome{}, err
	}
	if blocked != nil {
		return st.Outcome{Status: st.Failed, Code: "runtime.outcome-unknown", Targets: []st.TargetOutcome{*blocked}}, nil
	}
	target, uncertain := s.runImagePolicyTarget(ctx, p, row, previous.ID, "auto-update")
	target = qualifyUpdateTarget(row.EnvironmentID, row.TargetKind, row.TargetID, target)
	if err := jobcontext.Progress(ctx, target); err != nil {
		return st.Outcome{Status: st.Failed, Targets: []st.TargetOutcome{target}}, err
	}
	out := st.Outcome{Status: st.Succeeded, Targets: []st.TargetOutcome{target}}
	if uncertain {
		out.Status, out.Code = st.Failed, "runtime.outcome-unknown"
	}
	return policyResult(out), nil
}
