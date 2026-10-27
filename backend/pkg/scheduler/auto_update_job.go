// Source: getarcaneapp/arcane/backend/pkg/scheduler/auto_update_job.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; neutralise inherited triggers in favour of hub-owned stack policy jobs

package scheduler

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

type pendingUpdateApplierInternal interface {
	ApplyPending(context.Context, dto.Options) (*dto.Result, error)
}

// AutoUpdateJob is an inert legacy construction boundary. Only ImagePolicyJob
// may submit current stack runtime updates from the hub-owned scheduler.
type AutoUpdateJob struct {
	updaterService  pendingUpdateApplierInternal
	settingsService *settings.SettingsService
	admissionGate   *runs.Admission
}

func NewAutoUpdateJob(owner *updater.UpdaterService, settings *settings.SettingsService, gate *runs.Admission) (*AutoUpdateJob, error) {
	if gate == nil {
		return nil, errors.New("auto-update admission gate unavailable")
	}
	return &AutoUpdateJob{owner, settings, gate}, nil
}
func (*AutoUpdateJob) Name() string                        { return "auto-update" }
func (*AutoUpdateJob) ShouldSchedule(context.Context) bool { return false }
func (*AutoUpdateJob) Schedule(context.Context) string     { return "0 0 0 * * *" }
func (*AutoUpdateJob) Run(context.Context) (st.Outcome, error) {
	return st.Outcome{Status: st.Skipped, Code: "update.agent-local-job-disabled"}, nil
}
func (*AutoUpdateJob) Reconcile(context.Context, st.Run) (st.Outcome, error) {
	return st.Outcome{Status: st.Skipped, Code: "update.agent-local-job-disabled"}, nil
}
func (*AutoUpdateJob) ValidateRetry(context.Context, st.Run) error {
	return imageDTO.Failure("update.agent-local-job-disabled", 409)
}
func (*AutoUpdateJob) Reschedule(context.Context) error {
	return imageDTO.Failure("update.schedule-hub-owned", 409)
}
