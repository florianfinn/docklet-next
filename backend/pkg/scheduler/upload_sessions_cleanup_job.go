// Source: getarcaneapp/arcane/backend/pkg/scheduler/upload_sessions_cleanup_job.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package scheduler

import (
	"context"
	"log/slog"
	"time"

	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"

	"github.com/florianfinn/docklet-next/backend/internal/upload"
)

const (
	// UploadSessionsCleanupJobName identifies the hourly purge of idle chunked
	// upload sessions.
	UploadSessionsCleanupJobName = "upload-sessions-cleanup"

	uploadSessionMaxAge = 24 * time.Hour
)

// UploadSessionsCleanupJob purges upload sessions that have been idle for
// longer than uploadSessionMaxAge. It runs on managers and agents alike,
// since sessions live on whichever node serves the environment.
type UploadSessionsCleanupJob struct {
	uploadService *upload.UploadService
}

// NewUploadSessionsCleanupJob builds the cleanup job for the scheduler.
func NewUploadSessionsCleanupJob(uploadService *upload.UploadService) *UploadSessionsCleanupJob {
	return &UploadSessionsCleanupJob{uploadService: uploadService}
}

func (j *UploadSessionsCleanupJob) Name() string {
	return UploadSessionsCleanupJobName
}

func (j *UploadSessionsCleanupJob) Schedule(ctx context.Context) string {
	return "0 0 * * * *"
}

func (j *UploadSessionsCleanupJob) Run(ctx context.Context) (schedulertypes.Outcome, error) {
	removed, err := j.uploadService.PurgeExpiredSessions(ctx, uploadSessionMaxAge)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to purge expired upload sessions", "jobName", UploadSessionsCleanupJobName, "removed", removed, "error", err)
		return schedulertypes.Outcome{}, err
	}
	if removed > 0 {
		slog.InfoContext(ctx, "Purged expired upload sessions", "jobName", UploadSessionsCleanupJobName, "removed", removed)
	}
	return schedulertypes.Outcome{Status: schedulertypes.Succeeded}, nil
}

func (j *UploadSessionsCleanupJob) Reschedule(ctx context.Context) error {
	return nil
}
