// Source: getarcaneapp/arcane/backend/pkg/scheduler/auto_patch_job.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package scheduler

import (
	"cmp"
	"context"
	"errors"
	"log/slog"

	"github.com/florianfinn/docklet-next/types"
	"github.com/florianfinn/docklet-next/types/features"
	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/user"
	"go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	scheduleutil "github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
)

const AutoPatchJobName = "auto-patch"

var autoPatchSystemUser = user.Actor{
	Username: "System",
}

// AutoPatchJob periodically patches images whose latest vulnerability scan
// found fixable OS package vulnerabilities. It is opt-in via the
// "imageAutoPatchEnabled" setting.
type AutoPatchJob struct {
	imageService    *image.ImageService
	settingsService *settings.SettingsService
}

// NewAutoPatchJob creates a new AutoPatchJob.
func NewAutoPatchJob(imageService *image.ImageService, settingsService *settings.SettingsService) *AutoPatchJob {
	return &AutoPatchJob{
		imageService:    imageService,
		settingsService: settingsService,
	}
}

func (j *AutoPatchJob) Name() string {
	return AutoPatchJobName
}

func (j *AutoPatchJob) ShouldSchedule(ctx context.Context) bool {
	return j.settingsService.IsFeatureEnabled(ctx, features.VulnerabilityManagement) && j.settingsService.GetBoolSetting(ctx, "imageAutoPatchEnabled", false)
}

// Schedule returns the cron expression for the job. Defaults to daily at 3 AM.
func (j *AutoPatchJob) Schedule(ctx context.Context) string {
	schedule := cmp.Or(j.settingsService.GetStringSetting(ctx, "imageAutoPatchInterval", "0 0 3 * * *"), "0 0 3 * * *")

	parser := scheduleutil.Parser()
	if _, err := parser.Parse(schedule); err != nil {
		slog.WarnContext(ctx, "Invalid cron expression for auto-patch, using default", "invalidSchedule", schedule, "error", err)
		return "0 0 3 * * *"
	}

	return schedule
}

func (j *AutoPatchJob) Run(ctx context.Context) (schedulertypes.Outcome, error) {
	if !j.ShouldSchedule(ctx) {
		slog.DebugContext(ctx, "scheduled image patching disabled; skipping run")
		return schedulertypes.Outcome{Status: schedulertypes.Skipped}, nil
	}

	slog.InfoContext(ctx, "scheduled image patching started")

	patched, skipped, err := j.imageService.PatchFlaggedImages(ctx, types.LocalDockerEnvironmentID, autoPatchSystemUser)
	if errors.Is(err, common.ErrFeatureDisabled) {
		status := kit.Ternary(patched > 0, schedulertypes.Partial, schedulertypes.Skipped)
		return schedulertypes.Outcome{Status: status, Message: err.Error()}, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "scheduled image patching failed", "error", err)
		if patched > 0 {
			return schedulertypes.Outcome{Status: schedulertypes.Partial}, err
		}
		return schedulertypes.Outcome{}, err
	}

	slog.InfoContext(ctx, "scheduled image patching completed",
		"patched", patched,
		"skipped", skipped,
	)
	return schedulertypes.Outcome{Status: schedulertypes.Succeeded}, nil
}
