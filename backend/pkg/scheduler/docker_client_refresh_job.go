// Source: getarcaneapp/arcane/backend/pkg/scheduler/docker_client_refresh_job.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package scheduler

import (
	"cmp"
	"context"
	"log/slog"

	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	scheduleutil "github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
)

const (
	DockerClientRefreshJobName         = "docker-client-refresh"
	dockerClientRefreshDefaultSchedule = "0 */5 * * * *"
)

// DockerClientRefreshJob keeps the cached Docker client aligned with the daemon
// API version after daemon restarts or upgrades.
type DockerClientRefreshJob struct {
	dockerClientService *docker.DockerClientService
	settingsService     *settings.SettingsService
}

// NewDockerClientRefreshJob creates the scheduled Docker client refresh job.
func NewDockerClientRefreshJob(dockerClientService *docker.DockerClientService, settingsService *settings.SettingsService) *DockerClientRefreshJob {
	return &DockerClientRefreshJob{
		dockerClientService: dockerClientService,
		settingsService:     settingsService,
	}
}

func (j *DockerClientRefreshJob) Name() string {
	return DockerClientRefreshJobName
}

func (j *DockerClientRefreshJob) Schedule(ctx context.Context) string {
	schedule := cmp.Or(j.settingsService.GetStringSetting(ctx, "dockerClientRefreshInterval", dockerClientRefreshDefaultSchedule), dockerClientRefreshDefaultSchedule)

	parser := scheduleutil.Parser()
	if _, err := parser.Parse(schedule); err != nil {
		slog.WarnContext(ctx, "Invalid cron expression for Docker client refresh, using default", "invalidSchedule", schedule, "error", err)
		return dockerClientRefreshDefaultSchedule
	}

	return schedule
}

func (j *DockerClientRefreshJob) Run(ctx context.Context) (schedulertypes.Outcome, error) {
	if err := j.dockerClientService.RefreshClient(ctx); err != nil {
		slog.WarnContext(ctx, "Docker client refresh failed", "error", err)
		return schedulertypes.Outcome{}, err
	}

	slog.DebugContext(ctx, "Docker client refresh completed")
	return schedulertypes.Outcome{Status: schedulertypes.Succeeded}, nil
}
