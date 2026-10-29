// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package di

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

func provideAutoHealJobInternal(d *docker.DockerClientService, s *settings.SettingsService, e *event.EventService, n *notification.NotificationService, a *runs.Admission, db *database.DB, cfg *config.Config, runtime *activity.RuntimeService, volumes *volume.VolumeService) (*scheduler.AutoHealJob, error) {
	return scheduler.NewAutoHealJob(d, s, e, n, a, scheduler.AutoHealOptions{DB: db, Config: cfg, Busy: func(ctx context.Context, v container.InspectResponse) (bool, error) {
		held, err := runtime.HeldForContainer(ctx, v.ID, "")
		if held || err != nil {
			return true, err
		}
		names := []string{}
		for _, m := range v.Mounts {
			if m.Type == mount.TypeVolume && m.Name != "" {
				names = append(names, m.Name)
			}
		}
		return volumes.HeldForAutoHeal(ctx, names)
	}})
}
