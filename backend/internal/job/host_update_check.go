// Source: docklet-next/issues/597
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original hub-only host check admission and restart execution.

package job

import (
	"cmp"
	"context"
	"errors"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
	st "github.com/florianfinn/docklet-next/types/scheduler"
)

const hostUpdateCheckJob = "update-check-host"

func (s *JobService) hostUpdateCheckAccess(ctx context.Context, run st.Run) error {
	if s.cfg == nil || s.cfg.AgentMode || s.cfg.EdgeAgent {
		return imageDTO.Failure("update.schedule-hub-owned", 409)
	}
	if s.imagePolicies == nil {
		return imageDTO.Failure("update.check-unavailable", 503)
	}
	if s.db == nil {
		return imageDTO.Failure("update.check-unavailable", 503)
	}
	var host struct{ Enabled bool }
	if err := s.db.WithContext(ctx).Table("environments").Select("enabled").Where("id = ?", run.EnvironmentID).Take(&host).Error; err != nil || !host.Enabled {
		return imageDTO.Failure("agent.unavailable", 503)
	}
	return s.authorizeRunInternal(ctx, run)
}
func (s *JobService) submitHostUpdateCheck(ctx context.Context, request st.Request) (st.Run, error) {
	request.EnvironmentID = cmp.Or(request.EnvironmentID, "0")
	request.Trigger = cmp.Or(request.Trigger, "manual")
	if request.Trigger != "manual" {
		return st.Run{}, errors.New("host update checks require manual admission")
	}
	run := st.Run{EnvironmentID: request.EnvironmentID, Trigger: request.Trigger, RequestedBy: request.RequestedBy, RequestedWithKey: request.RequestedWithKey}
	if err := s.hostUpdateCheckAccess(ctx, run); err != nil {
		return st.Run{}, err
	}
	return s.runs.Submit(ctx, request)
}
func (s *JobService) executeHostUpdateCheck(ctx context.Context, run st.Run) (st.Outcome, error) {
	if err := s.hostUpdateCheckAccess(ctx, run); err != nil {
		return st.Outcome{Status: st.Failed, Code: "agent.unavailable"}, err
	}
	return s.imagePolicies.RunUpdateCheckHost(s.runContextInternal(ctx, run), run.EnvironmentID)
}
