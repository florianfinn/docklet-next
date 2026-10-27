// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Hub-owned logical job projection and controls, never agent forwarding.

package job

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/job/children/runtime"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	"github.com/florianfinn/docklet-next/types/meta"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"sort"
	"strings"
	"time"
)

func (s *JobService) SetImagePolicies(owner *updater.UpdaterService) { s.imagePolicies = owner }
func (s *JobService) imagePolicyJob(env, job string) (string, bool, error) {
	if strings.HasPrefix(job, "image-policy:") {
		return "", true, imageDTO.Failure("update.invalid-target", 400)
	}
	if !updater.IsImagePolicyJob(job) {
		return "", false, nil
	}
	if s.cfg.AgentMode || s.cfg.EdgeAgent {
		return "", true, imageDTO.Failure("update.agent-local-job-disabled", 409)
	}
	if s.imagePolicies == nil {
		return "", true, imageDTO.Failure("update.persistence-failed", 500)
	}
	return updater.ImagePolicyJobKey(env, job), true, nil
}
func projectImagePolicyRun(run st.Run) st.Run {
	if env, kind, owned := updater.ParseImagePolicyJob(run.JobID); owned {
		run.JobID = kind
		run.EnvironmentID = env
		if run.ActivityID != "" {
			run.ActivityEnvironmentID = env
		}
	}
	return run
}
func (s *JobService) Submit(ctx context.Context, request st.Request) (st.Run, error) {
	id, owned, err := s.imagePolicyJob(request.EnvironmentID, request.JobID)
	if err != nil {
		return st.Run{}, err
	}
	if !owned {
		return s.legacySubmit(ctx, request)
	}
	if s.roles == nil {
		return st.Run{}, huma.Error403Forbidden("Job actor unavailable")
	}
	grants, e := s.roles.ResolveExecutionPermissions(ctx, request.RequestedBy, request.RequestedWithKey)
	if e != nil || !grants.Allows(authz.PermJobsManage, request.EnvironmentID) {
		return st.Run{}, huma.Error403Forbidden("Job grants required")
	}
	if _, e = s.imagePolicies.ImagePolicySchedule(ctx, request.EnvironmentID); e != nil {
		return st.Run{}, e
	}
	request.JobID = id
	request.EnvironmentID = "0"
	return s.runs.Submit(ctx, request)
}
func (s *JobService) GetRun(ctx context.Context, env, job, run string) (st.Run, error) {
	id, owned, err := s.imagePolicyJob(env, job)
	if err != nil {
		return st.Run{}, err
	}
	if owned {
		return s.runs.Get(ctx, "0", id, run)
	}
	return s.legacyGetRun(ctx, env, job, run)
}
func (s *JobService) ListRuns(ctx context.Context, env, job string, page, limit int) (st.RunList, error) {
	id, owned, err := s.imagePolicyJob(env, job)
	if err != nil {
		return st.RunList{}, err
	}
	if owned {
		return s.runs.List(ctx, "0", id, page, limit)
	}
	return s.legacyListRuns(ctx, env, job, page, limit)
}
func (s *JobService) CancelRun(ctx context.Context, env, job, run string) (st.Run, error) {
	id, owned, err := s.imagePolicyJob(env, job)
	if err != nil {
		return st.Run{}, err
	}
	if owned {
		return s.runs.Cancel(ctx, "0", id, run)
	}
	return s.legacyCancelRun(ctx, env, job, run)
}
func (s *JobService) RetryRun(ctx context.Context, env, job, run string) (st.Run, error) {
	id, owned, err := s.imagePolicyJob(env, job)
	if err != nil {
		return st.Run{}, err
	}
	if owned {
		previous, e := s.runs.Get(ctx, "0", id, run)
		if e != nil {
			return previous, e
		}
		if previous.AttemptCount == 0 {
			return previous, imageDTO.Failure("update.invalid-target", 400)
		}
		// The next attempt is receipt-only reconciliation, never fresh admission.
		return s.runs.Retry(ctx, "0", id, run)
	}
	return s.legacyRetryRun(ctx, env, job, run)
}
func (s *JobService) ResolveRun(ctx context.Context, env, job, run, actor string) (st.Run, error) {
	id, owned, err := s.imagePolicyJob(env, job)
	if err != nil {
		return st.Run{}, err
	}
	if owned {
		grants, _ := middleware.PermissionsFromContext(ctx)
		if actor == "" || !grants.Allows(authz.PermJobsManage, env) {
			return st.Run{}, huma.Error403Forbidden("Job resolution grants required")
		}
		return s.runs.Resolve(ctx, "0", id, run, actor)
	}
	return s.legacyResolveRun(ctx, env, job, run, actor)
}
func filterImagePolicyRows(rows []jobschedule.JobStatus) []jobschedule.JobStatus {
	out := make([]jobschedule.JobStatus, 0, len(rows))
	for _, row := range rows {
		if updater.IsImagePolicyJob(row.ID) || strings.HasPrefix(row.ID, "image-policy:") {
			continue
		}
		out = append(out, row)
	}
	return out
}
func (s *JobService) ListJobs(ctx context.Context) (*jobschedule.JobListResponse, error) {
	result, err := s.legacyListJobs(ctx)
	if err != nil {
		return result, err
	}
	result.Jobs = filterImagePolicyRows(result.Jobs)
	if !(s.cfg.AgentMode || s.cfg.EdgeAgent) && s.imagePolicies != nil {
		cfg, e := s.ImagePolicySchedules(ctx, "0")
		if e != nil {
			return result, e
		}
		result.Jobs = append(result.Jobs, cfg.Jobs...)
	}
	sort.Slice(result.Jobs, func(i, j int) bool { return result.Jobs[i].ID < result.Jobs[j].ID })
	return result, nil
}
func (s *JobService) ListRemoteJobs(ctx context.Context, env string) (*jobschedule.JobListResponse, error) {
	result, err := s.legacyListRemoteJobs(ctx, env)
	if (s.cfg.AgentMode || s.cfg.EdgeAgent) || s.imagePolicies == nil {
		return result, err
	}
	cfg, e := s.ImagePolicySchedules(ctx, env)
	if e != nil {
		return result, e
	}
	if err != nil {
		result = &jobschedule.JobListResponse{DurableRuns: true, Offline: true, ObservedAt: time.Now().UTC(), Jobs: []jobschedule.JobStatus{}}
	}
	result.Jobs = append(filterImagePolicyRows(result.Jobs), cfg.Jobs...)
	sort.Slice(result.Jobs, func(i, j int) bool { return result.Jobs[i].ID < result.Jobs[j].ID })
	return result, nil
}
func (s *JobService) ImagePolicySchedules(ctx context.Context, env string) (dto.UpdateScheduleConfig, error) {
	if s.cfg.AgentMode || s.cfg.EdgeAgent {
		return dto.UpdateScheduleConfig{}, imageDTO.Failure("update.schedule-hub-owned", 409)
	}
	if s.imagePolicies == nil {
		return dto.UpdateScheduleConfig{}, imageDTO.Failure("update.persistence-failed", 500)
	}
	row, err := s.imagePolicies.ImagePolicySchedule(ctx, env)
	if err != nil {
		return dto.UpdateScheduleConfig{}, err
	}
	out := dto.UpdateScheduleConfig{Revision: row.Revision, EnvironmentID: env, RegistrationID: row.RegistrationID, PollingInterval: row.PollingInterval, AutoUpdateInterval: row.AutoUpdateInterval, PollingEnabled: row.PollingEnabled, AutoUpdateEnabled: row.AutoUpdateEnabled, MasterPollingEnabled: s.settings.GetBoolSetting(ctx, "pollingEnabled", true), MasterAutoUpdateEnabled: s.settings.GetBoolSetting(ctx, "autoUpdate", false), TimeZone: s.location.String(), Jobs: []jobschedule.JobStatus{}}
	for _, kind := range []string{"image-polling", "auto-update"} {
		metadata, _ := meta.GetJobMetadata(kind)
		cron := row.PollingInterval
		wireKind := "update-check"
		if kind == "auto-update" {
			cron = row.AutoUpdateInterval
			wireKind = "auto-update"
		}
		physical := updater.ImagePolicyJobKey(env, kind)
		enabled := s.imagePolicies.ImagePolicyEnabled(ctx, row, kind)
		status := metadata.ToJobStatus(cron, nil, enabled, []jobschedule.JobPrerequisite{})
		status.ID = kind
		status.HostID = env
		status.Kind = wireKind
		status.TimeZone = out.TimeZone
		status.ManagerOnly = true
		status.IsContinuous = false
		status.SettingsKey = ""
		status.CanRunManually = enabled
		if s.runs != nil {
			record, e := s.runs.ScheduleState(ctx, physical)
			if e == nil {
				runtime.ApplyRunStatus(&status, record.Runs)
				if enabled && !record.NextRun.IsZero() {
					status.NextRun = new(record.NextRun)
				}
			}
		}
		if status.CurrentRun != nil {
			projected := projectImagePolicyRun(*status.CurrentRun)
			status.CurrentRun = &projected
		}
		if status.LastRun != nil {
			projected := projectImagePolicyRun(*status.LastRun)
			status.LastRun = &projected
		}
		if !enabled {
			status.Code = "job.skipped"
			if row.ActorID == "" {
				status.Code = "update.schedule-owner-unset"
			}
		}
		out.Jobs = append(out.Jobs, status)
	}
	return out, nil
}
