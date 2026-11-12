// Source: getarcaneapp/arcane/backend/pkg/scheduler/event_cleanup_job.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; fixed event and finished-activity limits; M19 backend contracts and target identity integration.

package scheduler

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"log/slog"

	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
	kit "go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
)

const EventCleanupJobName = "event-cleanup"

type EventCleanupJob struct {
	eventService    *event.EventService
	activityService *activity.ActivityService
	settingsService *settings.SettingsService
}

func NewEventCleanupJob(eventService *event.EventService, activityService *activity.ActivityService, settingsService *settings.SettingsService) *EventCleanupJob {
	j := &EventCleanupJob{
		eventService:    eventService,
		activityService: activityService,
		settingsService: settingsService,
	}
	if settingsService != nil {
		settingsService.SubscribeSettingsChanges([]string{"activityEventLimit", "activityRunLimit"}, func(_ []libarcane.SettingUpdate) {
			_, err := j.Run(context.Background())
			if err != nil {
				slog.Warn("Activity retention after save failed", "error", err)
			}
		})
	}
	return j
}

func (j *EventCleanupJob) Name() string {
	return EventCleanupJobName
}

func (j *EventCleanupJob) Schedule(ctx context.Context) string {
	s := j.settingsService.GetStringSetting(ctx, "eventCleanupInterval", "0 0 */6 * * *")
	return kit.Ternary(s == "", "0 0 */6 * * *", s)
}

func (j *EventCleanupJob) Run(ctx context.Context) (schedulertypes.Outcome, error) {
	slog.InfoContext(ctx, "Running event cleanup job", "jobName", EventCleanupJobName)

	deletedEvents, err := j.eventService.PruneEvents(ctx, j.settingsService.GetIntSetting(ctx, "activityEventLimit", 500))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to prune events", "jobName", EventCleanupJobName, "error", err)
		return schedulertypes.Outcome{}, err
	}

	slog.InfoContext(ctx, "Event cleanup job completed successfully",
		"jobName", EventCleanupJobName,
		"maxEntries", j.settingsService.GetIntSetting(ctx, "activityEventLimit", 500),
		"deleted", deletedEvents)

	if j.activityService != nil {
		deleted, err := j.activityService.PruneHistory(ctx, j.settingsService.GetIntSetting(ctx, "activityRunLimit", 100))
		if err != nil {
			slog.ErrorContext(ctx, "Failed to prune activity history", "jobName", EventCleanupJobName, "error", err)
			return schedulertypes.Outcome{}, err
		}

		slog.InfoContext(ctx, "Activity history cleanup completed successfully",
			"jobName", EventCleanupJobName,
			"maxEntries", j.settingsService.GetIntSetting(ctx, "activityRunLimit", 100),
			"deleted", deleted)
	}
	return schedulertypes.Outcome{Status: schedulertypes.Succeeded}, nil
}

func (j *EventCleanupJob) Reschedule(ctx context.Context) error {
	slog.InfoContext(ctx, "rescheduling event cleanup job in new scheduler; currently requires restart")
	return nil
}
