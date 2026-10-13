// Source: getarcaneapp/arcane/backend/pkg/scheduler/apns_outbox_job.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package scheduler

import (
	"context"

	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"

	"github.com/florianfinn/docklet-next/backend/internal/apns"
	"github.com/florianfinn/docklet-next/backend/internal/config"
)

const ApnsOutboxJobName = "apns-outbox"

type ApnsOutboxJob struct {
	service *apns.ApnsService
	config  *config.Config
}

func NewApnsOutboxJob(service *apns.ApnsService, cfg *config.Config) *ApnsOutboxJob {
	return &ApnsOutboxJob{service: service, config: cfg}
}

func (j *ApnsOutboxJob) Name() string {
	return ApnsOutboxJobName
}

func (j *ApnsOutboxJob) Schedule(_ context.Context) string {
	return "0 */5 * * * *"
}

func (j *ApnsOutboxJob) ShouldSchedule(ctx context.Context) bool {
	return (j.config == nil || !j.config.AgentMode) && j.service.Enabled(ctx)
}

func (j *ApnsOutboxJob) Run(ctx context.Context) (schedulertypes.Outcome, error) {
	if err := j.service.DrainOutbox(ctx); err != nil {
		return schedulertypes.Outcome{}, err
	}
	return schedulertypes.Outcome{Status: schedulertypes.Succeeded}, nil
}
