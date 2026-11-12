// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Serial policy children through runtime admission; receipt-only restart recovery. M24 recovery evidence, validation and reason contracts. M24 window suppression binding and all-caller transactional child settlement.

package updater

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Masterminds/semver/v3"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	digest "github.com/opencontainers/go-digest"
	"gorm.io/gorm"
	"log/slog"
	"sort"
)

func policyTarget(id string, status st.RunStatus, code string) st.TargetOutcome {
	return st.TargetOutcome{ID: id, ResourceType: "project", Status: status, Code: code, Message: code}
}

// Update jobs aggregate targets across hosts and kinds. Consumers use the
// explicit identity fields; the qualified ID is the coordinator's merge key.
func qualifyUpdateTarget(env, kind, id string, out st.TargetOutcome) st.TargetOutcome {
	out.ID = env + "~" + kind + "~" + id
	out.EnvironmentID, out.TargetKind, out.TargetID = env, kind, id
	out.ResourceType = kind
	return out
}
func policyResult(outcome st.Outcome) st.Outcome {
	if outcome.Status == st.Failed {
		return outcome
	}
	for _, target := range outcome.Targets {
		if target.Status == st.Failed {
			outcome.Status = st.Partial
			break
		}
	}
	return outcome
}
func (s *UpdaterService) runImagePolicyTarget(ctx context.Context, p *imagePolicyService, one policy.Policy, jobRun, job string) (st.TargetOutcome, bool) {
	target, uncertain := s.executeImagePolicyTarget(ctx, p, one, jobRun, job)
	return s.recordImagePolicyStatus(ctx, p, one, job, target), uncertain
}
func (s *UpdaterService) recordImagePolicyStatus(ctx context.Context, p *imagePolicyService, one policy.Policy, job string, target st.TargetOutcome) st.TargetOutcome {
	if target.Code == "update.permission-denied" {
		target.Status = st.Failed
	}
	if job == "auto-update" && one.Mode != "scheduled" {
		return target
	}
	changes := map[string]any{"code": target.Code}
	if job == "auto-update" {
		changes = map[string]any{"apply_code": target.Code, "apply_checked_at": policyNow()}
	} else if target.Status == st.Succeeded {
		changes["checked_at"] = policyNow()
	}
	if err := p.store.DB.WithContext(ctx).Model(&policy.Policy{}).Where("id = ? AND registration_id = ? AND revision = ?", one.ID, one.RegistrationID, one.Revision).Updates(changes).Error; err != nil {
		target.Status = st.Failed
		target.Code = "update.persistence-failed"
		target.Message = target.Code
	}
	return target
}
func (s *UpdaterService) executeImagePolicyTarget(ctx context.Context, p *imagePolicyService, one policy.Policy, jobRun, job string) (st.TargetOutcome, bool) {
	skip := func(code string) (st.TargetOutcome, bool) { return policyTarget(one.TargetID, st.Skipped, code), false }
	if job == "auto-update" && one.Mode != "scheduled" {
		return skip("job.skipped")
	}
	actorCtx := ctx
	var err error
	if job == "auto-update" {
		actorCtx, err = p.ports.actor(ctx, one.EnvironmentID, one.ActorID, one.ActorKeyID, true)
		if err != nil {
			return policyTarget(one.TargetID, st.Failed, "update.permission-denied"), false
		}
	}
	reg, err := p.ports.host(ctx, one.EnvironmentID)
	if err != nil || reg != one.RegistrationID {
		return skip("update.target-changed")
	}
	var baseline dto.BaselineStatus
	var resolved updateTarget
	var check *projectDTO.UpdateInfo
	if one.TargetKind == "container" {
		resolved, err = p.findContainerTarget(ctx, one.EnvironmentID, one.TargetID)
		if err != nil {
			return skip(policyCode(err, "update.check-unavailable"))
		}
		if job == "auto-update" && resolved.AutoExcluded {
			return skip("update.auto-excluded")
		}
		check, err = p.ports.checkContainer(ctx, one.EnvironmentID, resolved)
	} else {
		baseline, err = p.ports.baseline(ctx, one.EnvironmentID, one.TargetID)
		if job == "auto-update" {
			if err != nil {
				return skip(policyCode(err, "runtime.target-unavailable"))
			}
			if baseline.TargetIdentity != one.TargetIdentity {
				return skip("update.target-changed")
			}
			if baseline.AutoExcluded {
				return skip("update.auto-excluded")
			}
			if !baseline.Available {
				code := baseline.Code
				if code == "" {
					code = "update.baseline-unavailable"
				}
				return skip(policyCode(imageDTO.Failure(code, 409), "update.persistence-failed"))
			}
		}
		check, err = p.ports.check(ctx, one.EnvironmentID, one.TargetID)
	}
	if err != nil || check == nil {
		return skip(policyCode(err, "update.check-unavailable"))
	}
	if check.Status == "failed" || check.Status == "unknown" || check.Status == "error" || check.Status == "not_pulled" {
		return skip("update.check-unavailable")
	}
	candidates, err := s.observeImagePolicy(ctx, p, one, check)
	if err != nil {
		return policyTarget(one.TargetID, st.Failed, policyCode(err, "update.persistence-failed")), false
	}
	if job != "auto-update" || len(candidates) == 0 {
		return policyTarget(one.TargetID, st.Succeeded, ""), false
	}
	actorCtx, err = p.ports.actor(ctx, one.EnvironmentID, one.ActorID, one.ActorKeyID, true)
	if err != nil {
		return policyTarget(one.TargetID, st.Failed, "update.permission-denied"), false
	}
	child, fresh, err := p.store.PrepareChild(actorCtx, one, jobRun)
	if err != nil {
		return policyTarget(one.TargetID, st.Failed, policyCode(err, "update.persistence-failed")), false
	}
	if !fresh {
		return s.reconcileImagePolicyChild(ctx, p, one.EnvironmentID, one.TargetID, child)
	}
	binding := dto.PolicyBinding{PolicyID: one.ID, Revision: one.Revision, JobRunID: jobRun, ChildID: child.ID}
	binding.Suppressed, err = p.store.FailedCandidates(actorCtx, one.ID, one.TargetIdentity)
	if err != nil {
		return policyTarget(one.TargetID, st.Failed, "update.persistence-failed"), false
	}
	var accepted activity.RuntimeAdmissionResult
	if one.TargetKind == "container" {
		accepted, err = p.ports.admitContainer(actorCtx, one.EnvironmentID, resolved.ContainerID, child.ID, binding)
	} else {
		accepted, err = p.ports.admit(actorCtx, one.EnvironmentID, one.TargetID, child.ID, dto.LifecycleRequest{ExpectedRevision: baseline.SavedRevision}, binding)
	}
	if err != nil {
		code := policyCode(err, "update.persistence-failed")
		// An admission storage failure can be after commit. A receipt wins over
		// the transport return; only proven absence is a zero-effect refusal.
		snapshot, found, e := p.ports.snapshot(ctx, one.EnvironmentID, child.ID)
		if e != nil {
			return policyTarget(one.TargetID, st.Failed, "runtime.outcome-unknown"), true
		}
		if found {
			return s.settleImagePolicySnapshot(ctx, p, one.TargetID, child, snapshot)
		}
		changed := p.store.DB.WithContext(ctx).Model(&policy.Child{}).Where("id = ? AND status = ?", child.ID, "prepared").Updates(map[string]any{"status": "refused", "code": code, "settled_at": policyNow()})
		if changed.Error != nil {
			return policyTarget(one.TargetID, st.Failed, "update.persistence-failed"), false
		}
		return skip(code)
	}
	if accepted.ActivityID != child.ID {
		return policyTarget(one.TargetID, st.Failed, "runtime.outcome-unknown"), true
	}
	snapshot, err := p.ports.wait(actorCtx, one.EnvironmentID, accepted.ActivityID)
	if err != nil {
		return policyTargetWithActivity(one.TargetID, accepted.ActivityID, st.Failed, "runtime.outcome-unknown"), true
	}
	return s.settleImagePolicySnapshot(ctx, p, one.TargetID, child, snapshot)
}

// A failed wrapper cannot release another runtime child's host-wide sequencing
// boundary. Only a known terminal receipt settles this checkpoint; no dispatch.
func (s *UpdaterService) imagePolicyHostHold(ctx context.Context, p *imagePolicyService, env, registration string) (*st.TargetOutcome, error) {
	var children []struct {
		policy.Child
		TargetID   string
		TargetKind string
	}
	err := p.store.DB.WithContext(ctx).Model(&policy.Child{}).Select("image_policy_children.*, update_policies.target_id, update_policies.target_kind").Joins("JOIN update_policies ON update_policies.id = image_policy_children.policy_id").Where("update_policies.environment_id = ? AND update_policies.registration_id = ? AND image_policy_children.status = ?", env, registration, "admitted").Order("image_policy_children.id").Scan(&children).Error
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		target, unknown := s.reconcileImagePolicyChild(ctx, p, env, child.TargetID, child.Child)
		if unknown {
			target = qualifyUpdateTarget(env, child.TargetKind, child.TargetID, target)
			return &target, nil
		}
	}
	return nil, nil
}
func policyTargetWithActivity(target, id string, status st.RunStatus, code string) st.TargetOutcome {
	out := policyTarget(target, status, code)
	out.ActivityID = id
	return out
}
func imagePolicySnapshotTarget(target, id string, snapshot activityDTO.RuntimeSnapshot) (st.TargetOutcome, bool) {

	code := ""
	unknown := func() (st.TargetOutcome, bool) {
		return policyTargetWithActivity(target, id, st.Failed, "runtime.outcome-unknown"), true
	}
	if snapshot.OutcomeState == activityDTO.RuntimeOutcomeUnknown || snapshot.Status != activityDTO.StatusSuccess && snapshot.Status != activityDTO.StatusFailed && snapshot.Status != activityDTO.StatusCancelled {
		return unknown()
	}
	lifecycle := snapshot.ImageLifecycle
	if lifecycle != nil {
		code = lifecycle.Code
		if imageDTO.ValidateReason(code) != nil || code == "runtime.outcome-unknown" {
			return unknown()
		}
	}
	if snapshot.MessageCode == "runtime.interrupted" || lifecycle != nil && lifecycle.Recovery != nil && lifecycle.Recovery.Outcome == "interrupted" {
		return policyTargetWithActivity(target, id, st.Failed, "update.interrupted"), false
	}
	if snapshot.Status == activityDTO.StatusCancelled {
		return policyTargetWithActivity(target, id, st.Canceled, code), false
	}
	if lifecycle == nil {
		return unknown()
	}
	status := st.Failed
	if snapshot.Status == activityDTO.StatusSuccess {
		status = st.Succeeded
		code = ""
	}
	if lifecycle.Recovery != nil {
		switch lifecycle.Recovery.Outcome {
		case "restored":
			code = "update.auto-restored"
		case "restore-failed":
			code = "update.auto-restore-failed"
		case "unavailable":
			code = "update.auto-restore-unavailable"
		}
	}
	if code == "image.busy" || code == "update.candidate-failed" {
		status = st.Skipped
	}

	return policyTargetWithActivity(target, id, status, code), false
}
func (s *UpdaterService) reconcileImagePolicyChild(ctx context.Context, p *imagePolicyService, env, target string, child policy.Child) (st.TargetOutcome, bool) {
	if child.Status == "refused" {
		return policyTarget(target, st.Skipped, child.Code), false
	}
	id := child.RuntimeID
	if id == "" {
		id = child.ID
	}
	snapshot, found, err := p.ports.snapshot(ctx, env, id)
	if err != nil {
		return policyTargetWithActivity(target, id, st.Failed, "runtime.outcome-unknown"), true
	}
	if !found {
		if child.Status == "prepared" {
			return policyTarget(target, st.Skipped, "runtime.interrupted"), false
		}
		return policyTargetWithActivity(target, id, st.Failed, "runtime.outcome-unknown"), true
	}
	return s.settleImagePolicySnapshot(ctx, p, target, child, snapshot)
}
func (s *UpdaterService) ReconcileImagePolicyHost(ctx context.Context, env string, previous st.Run) (st.Outcome, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return st.Outcome{Status: st.Skipped, Code: "update.agent-local-job-disabled"}, nil
	}
	var children []policy.Child
	if err = p.store.DB.WithContext(ctx).Where("job_run_id = ?", previous.ID).Order("id").Find(&children).Error; err != nil {
		return st.Outcome{Status: st.Failed, Code: "update.persistence-failed"}, nil
	}
	out := st.Outcome{Status: st.Failed, Code: "runtime.interrupted", Targets: previous.Outcome.Targets}
	if len(children) == 0 && len(out.Targets) == 0 {
		out.Status = st.Skipped
	}
	for _, child := range children {
		var one policy.Policy
		if err = p.store.DB.WithContext(ctx).First(&one, "id = ?", child.PolicyID).Error; err != nil {
			continue
		}
		target, _ := s.reconcileImagePolicyChild(ctx, p, env, one.TargetID, child)
		target = qualifyUpdateTarget(env, one.TargetKind, one.TargetID, target)
		replaced := false
		for i := range out.Targets {
			if out.Targets[i].ID == target.ID || out.Targets[i].EnvironmentID == "" && out.Targets[i].ID == one.TargetID {
				out.Targets[i] = target
				replaced = true
			}
		}
		if !replaced {
			out.Targets = append(out.Targets, target)
		}
	}
	return out, nil
}
func (s *UpdaterService) observeImagePolicy(ctx context.Context, p *imagePolicyService, one policy.Policy, check *projectDTO.UpdateInfo) ([]dto.UpdateCandidate, error) {
	candidates := []dto.UpdateCandidate{}
	keys := make([]string, 0, len(check.ServiceUpdates))
	for k := range check.ServiceUpdates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, service := range keys {
		row := check.ServiceUpdates[service]
		info := row.UpdateInfo
		if info == nil || !info.HasUpdate || info.State != "version" && info.State != "rebuild" {
			continue
		}
		named, err := reference.ParseNormalizedNamed(row.ImageRef)
		platform, platformErr := identity.ParsePlatform(info.Platform)
		candidateDigest, digestErr := digest.Parse(info.LatestDigest)
		if err != nil || platformErr != nil || digestErr != nil || candidateDigest.Algorithm() != digest.SHA256 {
			return nil, imageDTO.Failure("update.check-unavailable", 503)
		}
		version := info.LatestVersion
		if version == "" {
			if tagged, ok := named.(reference.Tagged); ok {
				version = tagged.Tag()
			}
		}
		if parsed, e := semver.NewVersion(version); e == nil {
			version = parsed.String()
		}
		c := dto.UpdateCandidate{Service: service, ImageRef: reference.TagNameOnly(named).String(), Version: version, Digest: candidateDigest.String(), Platform: platform.String()}
		c, err = p.store.RecordCandidate(ctx, one, c, s.deps.Settings.GetBoolSetting(ctx, "updateNotify", false))
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	payload, err := json.Marshal(candidates)
	if err != nil {
		return nil, err
	}
	changed := p.store.DB.WithContext(ctx).Model(&policy.Policy{}).Where("id = ? AND registration_id = ? AND revision = ?", one.ID, one.RegistrationID, one.Revision).Updates(map[string]any{"candidates": string(payload)})
	if changed.Error != nil {
		return nil, changed.Error
	}
	if changed.RowsAffected != 1 {
		return nil, imageDTO.Failure("update.policy-changed", 409)
	}
	if s.deps.Settings.GetBoolSetting(ctx, "updateNotify", false) {
		for i, c := range candidates {
			if c.EntryID != "" {
				c, err = s.deliverImagePolicyEntry(ctx, p, c)
				if err != nil {
					return nil, err
				}
				candidates[i] = c
			}
		}
		payload, err = json.Marshal(candidates)
		if err != nil {
			return nil, err
		}
		if err = p.store.DB.WithContext(ctx).Model(&policy.Policy{}).Where("id = ? AND registration_id = ? AND revision = ?", one.ID, one.RegistrationID, one.Revision).Update("candidates", string(payload)).Error; err != nil {
			return nil, err
		}
	}
	return candidates, nil
}
func (s *UpdaterService) deliverImagePolicyEntry(ctx context.Context, p *imagePolicyService, c dto.UpdateCandidate) (dto.UpdateCandidate, error) {
	var notice policy.Notice
	err := p.store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		changed := tx.Model(&policy.Notice{}).Where("id = ? AND status = ?", c.EntryID, "pending").Update("status", "sending")
		if changed.Error != nil {
			return changed.Error
		}
		if err := tx.First(&notice, "id = ?", c.EntryID).Error; err != nil {
			return err
		}
		if changed.RowsAffected == 0 {
			notice.Payload = ""
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	if notice.Payload != "" {
		count := 0
		var sendErr error
		if p.ports.notify != nil {
			count, sendErr = p.ports.notify(ctx, notice.EnvironmentID, notice.ID, c)
		}
		status := "sent"
		if sendErr != nil {
			status = "failed"
		} else if count == 0 {
			status = "unconfigured"
		}
		err = p.store.DB.WithContext(ctx).Model(&policy.Notice{}).Where("id = ? AND status = ?", notice.ID, "sending").Updates(map[string]any{"status": status, "delivered": count}).Error
		if err != nil {
			return c, err
		}
		notice.Status = status
	}
	c.NotificationState = notice.Status
	return c, nil
}

// CheckPolicyProject is the public/manual check consumer. Executor checks remain
// observations only; all policy notifications are deduplicated on the hub.
func (s *UpdaterService) CheckPolicyProject(ctx context.Context, env, target string) (*projectDTO.UpdateInfo, error) {
	if s.config != nil && (s.config.AgentMode || s.config.EdgeAgent) {
		return s.CheckProjectUpdates(ctx, target)
	}
	p, err := s.hubPolicies()
	if err != nil {
		return nil, err
	}
	one, fenceErr := s.manualPolicyFence(ctx, p, env, target)
	check, err := p.ports.check(ctx, env, target)
	var transport *remenv.TransportError
	if errors.As(err, &transport) {
		slog.WarnContext(ctx, "Update check could not reach the environment", "environment", env, "project", target, "error", err)
		return nil, imageDTO.Failure("update.check-unavailable", 503)
	}
	if err != nil {
		return nil, err
	}
	if check == nil || check.Status == "failed" || check.Status == "unknown" || check.Status == "error" || check.Status == "not_pulled" {
		return check, imageDTO.Failure("update.check-unavailable", 503)
	}
	result := *check
	result.PolicyCode = ""
	if fenceErr == nil {
		fenceErr = s.bookkeepManualPolicyRow(ctx, p, one, check)
	}
	if fenceErr != nil {
		result.PolicyCode = policyCode(fenceErr, "update.persistence-failed")
	}
	return &result, nil
}

func (s *UpdaterService) manualPolicyFence(ctx context.Context, p *imagePolicyService, env, target string) (policy.Policy, error) {
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return policy.Policy{}, err
	}
	one, err := p.store.Policy(ctx, env, reg, target)
	if err != nil {
		return one, err
	}
	if one.Revision == 0 {
		if baseline, e := p.ports.baseline(ctx, env, target); e == nil {
			one.TargetIdentity = baseline.TargetIdentity
		}
		return p.store.EnsureObservedPolicy(ctx, one)
	}
	return one, nil
}
func (s *UpdaterService) bookkeepManualPolicyCheck(ctx context.Context, p *imagePolicyService, env, target string, check *projectDTO.UpdateInfo) error {
	one, err := s.manualPolicyFence(ctx, p, env, target)
	if err != nil {
		return err
	}
	return s.bookkeepManualPolicyRow(ctx, p, one, check)
}
func (s *UpdaterService) bookkeepManualPolicyRow(ctx context.Context, p *imagePolicyService, one policy.Policy, check *projectDTO.UpdateInfo) (err error) {
	defer func() {
		code := ""
		if err != nil {
			code = policyCode(err, "update.persistence-failed")
		}
		// The observation succeeded independently of optional candidate bookkeeping.
		changed := p.store.DB.WithContext(ctx).Model(&policy.Policy{}).Where("id = ? AND registration_id = ? AND revision = ?", one.ID, one.RegistrationID, one.Revision).Updates(map[string]any{"checked_at": policyNow(), "code": "", "bookkeeping_code": code, "bookkeeping_at": policyNow()})
		if changed.Error != nil {
			err = imageDTO.Failure("update.persistence-failed", 500)
		}
	}()
	_, err = s.observeImagePolicy(ctx, p, one, check)
	return err
}
