// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Hub-owned logical job projection and controls, never agent forwarding.

package job

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	st "github.com/florianfinn/docklet-next/types/scheduler"
)

func (s *JobService) SetImagePolicies(owner *updater.UpdaterService) { s.imagePolicies = owner }
func projectImagePolicyRun(run st.Run) st.Run                        { return run }
func (s *JobService) Submit(ctx context.Context, r st.Request) (st.Run, error) {
	if err := retiredUpdateControl(r.JobID); err != nil {
		return st.Run{}, err
	}
	return s.legacySubmit(ctx, r)
}
func (s *JobService) GetRun(ctx context.Context, env, job, run string) (st.Run, error) {
	return s.legacyGetRun(ctx, env, job, run)
}
func (s *JobService) ListRuns(ctx context.Context, env, job string, page, limit int) (st.RunList, error) {
	return s.legacyListRuns(ctx, env, job, page, limit)
}
func (s *JobService) CancelRun(ctx context.Context, env, job, run string) (st.Run, error) {
	return s.legacyCancelRun(ctx, env, job, run)
}
func (s *JobService) RetryRun(ctx context.Context, env, job, run string) (st.Run, error) {
	if err := retiredUpdateControl(job); err != nil {
		return st.Run{}, err
	}
	return s.legacyRetryRun(ctx, env, job, run)
}
func (s *JobService) ResolveRun(ctx context.Context, env, job, run, actor string) (st.Run, error) {
	return s.legacyResolveRun(ctx, env, job, run, actor)
}
func (s *JobService) ListJobs(ctx context.Context) (*jobschedule.JobListResponse, error) {
	return s.legacyListJobs(ctx)
}
func (s *JobService) ListRemoteJobs(ctx context.Context, env string) (*jobschedule.JobListResponse, error) {
	return s.legacyListRemoteJobs(ctx, env)
}
