// Source: docklet-next/issues/567
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original executor-owned deploy progress observation and behavioural coverage.

package activity

import (
	"context"
	"log/slog"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
)

// The runtime owner supplies both the ID and its write context. Observation
// failure is diagnostic only and cannot change the deployment's outcome.
func (s *RuntimeService) deployReporter(ctx context.Context, runID string) (context.Context, func()) {
	reporter := projects.NewDeployReporter(func(phase string, warning bool) {
		message := "Deploy"
		if phase == "pull" {
			message = "Pull"
		}
		level := activitytypes.MessageLevel("info")
		if warning {
			message, level = "Preliminary pull failed; continuing deployment", "warning"
		}
		if _, err := s.activity.AppendMessages(ctx, runID, []AppendActivityMessageRequest{{Message: message, Step: phase, Level: level}}); err != nil {
			slog.WarnContext(ctx, "deploy phase persistence failed", "error", err)
		}
	}, func(patch map[string]any) {
		lock := s.activity.publishLockInternal(runID)
		lock.Lock()
		defer lock.Unlock()
		if err := s.activity.PatchActivityMetadata(ctx, runID, database.JSON(patch)); err != nil {
			slog.WarnContext(ctx, "deploy observation persistence failed", "error", err)
			return
		}
		var row Activity
		readCtx, cancel := ownerWriteContextInternal(ctx)
		defer cancel()
		if err := s.activity.db.WithContext(readCtx).First(&row, "id = ?", runID).Error; err != nil {
			slog.WarnContext(ctx, "deploy observation publication read failed", "error", err)
			return
		}
		s.activity.publishActivityInternal(activityToDTOInternal(&row))
	}, time.Now)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				reporter.Flush(false)
			case <-stop:
				return
			}
		}
	}()
	return projects.WithDeployReporter(ctx, reporter), func() { close(stop); <-done; reporter.Flush(true) }
}
