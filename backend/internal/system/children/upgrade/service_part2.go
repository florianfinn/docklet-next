// Source: getarcaneapp/arcane/backend/internal/system/children/upgrade/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package upgrade

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	imageDTO "github.com/florianfinn/docklet-next/types/image"

	"io/fs"
	"log/slog"
	"net/http"
	"os"

	"slices"
	"strings"

	"time"

	"github.com/containerd/errdefs"

	versiontypes "github.com/florianfinn/docklet-next/types/version"
	"github.com/moby/moby/api/types/container"

	"github.com/moby/moby/client"

	"github.com/florianfinn/docklet-next/backend/internal/database"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
)

// watchManagerUpgraderInternal closes out a job whose manager never restarted. The
// upgrader skips the container recreate when the pull lands on the image already
// running, so this process survives it — and the job, persisted pending_restart for
// the next boot to finalize, would otherwise wait for a restart that never comes.
//
// Runs at the tail of the agents-phase goroutine. When the manager IS recreated this
// process is stopped instead and ResumeUpdateAllOnStartup finalizes on the next boot.
// wasAlreadyNewest is the pre-trigger version check, the only evidence that a run
// which changed nothing was supposed to change nothing.
func (s *Service) watchManagerUpgraderInternal(ctx context.Context, jobID, upgraderID string, wasAlreadyNewest bool) {
	exit, exitCode := s.waitForUpgraderExitInternal(ctx, upgraderID)
	if exit == upgraderExitUnobservedInternal {
		// Never saw it finish, so a recreate may still be coming — but it may equally
		// never come, and pending_restart is only ever resolved at boot. Wait the run
		// out rather than returning: an unobserved exit must not strand the job.
		s.closeOutUnobservedUpgradeInternal(ctx, jobID)
		return
	}

	// A recreate stops this container, but not instantly — the stop carries a grace
	// period during which this goroutine still runs. Wait it out so a restart that is
	// already underway wins: if the manager is being replaced, the process dies here.
	time.Sleep(updateAllManagerNoRestartGraceInternal)

	job, err := s.getUpdateAllJobByIDInternal(ctx, jobID)
	if err != nil || job == nil {
		slog.WarnContext(ctx, "update-all: failed to reload job after upgrader exit", "jobId", jobID, "error", err)
		return
	}
	if job.Status != EnvironmentUpdateJobStatusPendingRestart {
		return
	}

	if exit == upgraderExitFailedInternal {
		s.markUpdateAllFailedInternal(ctx, job, fmt.Sprintf("manager upgrade failed: upgrader exited with code %d", exitCode))
		return
	}

	info := s.versionService.GetAppVersionInfo(ctx)

	// Still running here means the container was never recreated, so the upgrader either
	// found the image already current or died before it got that far. With no exit code
	// to tell those apart, a no-op is only credible when the pre-trigger check already
	// said there was nothing to swap in and the post-run check still agrees: an after-
	// the-fact "already newest" on its own restates what was true before the run, so it
	// cannot vouch for it — and on a manager that was due an update it would launder a
	// dead upgrader (stale newest-image state, a check that regressed to the running
	// image) into a green row and zero logged failures, leaving a broken upgrade with no
	// retry path. Anything less is a failure; the job closes either way, since no restart
	// is coming.
	if exit == upgraderExitCodeLostInternal && (!wasAlreadyNewest || !info.AlreadyOnNewest()) {
		s.markUpdateAllFailedInternal(ctx, job, "manager upgrade could not be confirmed: the upgrader exited without a readable status and this manager was not a confirmed no-op upgrade")
		return
	}

	s.recordManagerResultInternal(job, EnvironmentUpdateResultStatusUpToDate, info.CurrentVersion)
	slog.InfoContext(ctx, "Update-all: manager was already up to date; no restart needed", "jobId", job.ID, "version", info.CurrentVersion)
	s.finalizeUpdateAllJobInternal(ctx, job)
}

// closeOutUnobservedUpgradeInternal fails a job whose upgrader outcome was never
// observed and whose manager then never restarted. Waiting is what makes the answer
// safe: a recreate kills this process, so still being here once the job would be
// considered stale proves no restart is coming. Nothing else would close it — only a
// boot resolves pending_restart — and until it closes, every later update-all is
// refused as already in progress.
func (s *Service) closeOutUnobservedUpgradeInternal(ctx context.Context, jobID string) {
	job, err := s.getUpdateAllJobByIDInternal(ctx, jobID)
	if err != nil || job == nil {
		slog.WarnContext(ctx, "update-all: failed to reload job after unobserved upgrader exit", "jobId", jobID, "error", err)
		return
	}
	if job.Status != EnvironmentUpdateJobStatusPendingRestart {
		return
	}

	// Hold until the job hits the same staleness bound a resumed job is judged by, so a
	// slow upgrader still gets its full window to recreate this container.
	if remaining := time.Until(job.CreatedAt.Add(updateAllStaleThresholdInternal)); remaining > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(remaining):
		}
	}

	job, err = s.getUpdateAllJobByIDInternal(ctx, jobID)
	if err != nil || job == nil {
		slog.WarnContext(ctx, "update-all: failed to reload stale job", "jobId", jobID, "error", err)
		return
	}
	if job.Status != EnvironmentUpdateJobStatusPendingRestart {
		return
	}

	slog.WarnContext(ctx, "update-all: manager upgrade was never observed and no restart followed", "jobId", job.ID)
	s.markUpdateAllFailedInternal(ctx, job, "manager upgrade could not be observed: the upgrader run was never seen to finish and the manager did not restart")
}

// upgraderExitInternal is how much the watcher managed to learn about an upgrader run.
type upgraderExitInternal int

// waitForUpgraderExitInternal blocks until the upgrader container stops, reporting what
// could be learned about how it ended along with its exit code when one was observed.
func (s *Service) waitForUpgraderExitInternal(ctx context.Context, upgraderID string) (upgraderExitInternal, int64) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		slog.WarnContext(ctx, "update-all: cannot watch upgrader container", "upgraderId", upgraderID, "error", err)
		return upgraderExitUnobservedInternal, 0
	}

	waitCtx, cancel := context.WithTimeout(ctx, updateAllManagerWatchTimeoutInternal)
	defer cancel()

	wait := dockerClient.ContainerWait(waitCtx, upgraderID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})
	select {
	case result, ok := <-wait.Result:
		// A closed channel hands back a zero-value response, which is indistinguishable
		// from a clean exit 0 — nothing was delivered, so nothing may be concluded.
		if !ok {
			slog.WarnContext(ctx, "update-all: upgrader wait ended without a status", "upgraderId", upgraderID)
			return upgraderExitUnobservedInternal, 0
		}
		if result.StatusCode != 0 {
			return upgraderExitFailedInternal, result.StatusCode
		}
		// The daemon fills in status code 0 alongside a wait error (the container was
		// removed mid-wait, the wait itself broke): the exit is real but its code is not.
		if result.Error != nil {
			slog.WarnContext(ctx, "update-all: upgrader wait reported an error", "upgraderId", upgraderID, "error", result.Error.Message)
			return upgraderExitCodeLostInternal, 0
		}
		return upgraderExitSucceededInternal, 0
	case waitUpgraderErr, ok := <-wait.Error:
		// The upgrader runs with AutoRemove, so a fast run can be gone before the wait
		// is acknowledged. It exited, but the code went with it — a failed upgrade must
		// not be mistaken for a clean no-op, so report the ambiguity rather than a code.
		if errdefs.IsNotFound(waitUpgraderErr) {
			return upgraderExitCodeLostInternal, 0
		}
		if !ok || waitUpgraderErr == nil {
			slog.WarnContext(ctx, "update-all: upgrader wait ended without a status", "upgraderId", upgraderID)
		} else {
			slog.WarnContext(ctx, "update-all: failed waiting for upgrader container", "upgraderId", upgraderID, "error", waitUpgraderErr)
		}
		return upgraderExitUnobservedInternal, 0
	case <-waitCtx.Done():
		slog.WarnContext(ctx, "update-all: timed out waiting for upgrader container", "upgraderId", upgraderID)
		return upgraderExitUnobservedInternal, 0
	}
}

// seedRemoteResultsInternal builds a pending result row for every remote environment
// so the dialog can render the whole fleet immediately. Best effort: on error it
// returns nil and the agents phase appends rows as it processes them.
func (s *Service) seedRemoteResultsInternal(ctx context.Context, env *environment.EnvironmentService) EnvironmentUpdateResults {
	envs, err := env.ListRemoteEnvironments(ctx)
	if err != nil {
		slog.WarnContext(ctx, "update-all: failed to pre-list remote environments for seeding", "error", err)
		return nil
	}
	results := make(EnvironmentUpdateResults, 0, len(envs))
	for _, remote := range envs {
		results = append(results, EnvironmentUpdateResult{
			EnvironmentID:   remote.ID,
			EnvironmentName: remote.Name,
			Status:          EnvironmentUpdateResultStatusPending,
		})
	}
	return results
}

// upsertPendingResultInternal returns the index of the existing result row for envID,
// appending a new pending row when seeding missed it (e.g. the seed list failed, or a
// new environment was registered after the job started).
func upsertPendingResultInternal(job *EnvironmentUpdateJob, envID, envName string) int {
	if idx := slices.IndexFunc(job.Results, func(r EnvironmentUpdateResult) bool { return r.EnvironmentID == envID }); idx >= 0 {
		return idx
	}
	job.Results = append(job.Results, EnvironmentUpdateResult{
		EnvironmentID:   envID,
		EnvironmentName: envName,
		Status:          EnvironmentUpdateResultStatusPending,
	})
	return len(job.Results) - 1
}

// managerResultInternal returns the manager's (env "0") row, or nil when the job has none.
func managerResultInternal(job *EnvironmentUpdateJob) *EnvironmentUpdateResult {
	idx := slices.IndexFunc(job.Results, func(r EnvironmentUpdateResult) bool { return r.EnvironmentID == environment.LocalEnvironmentID })
	if idx < 0 {
		return nil
	}
	return &job.Results[idx]
}

// upgradeAgentInternal triggers and confirms a single remote environment's
// self-upgrade, recording the outcome on result. The upgrade always runs — the
// agent pulls the latest image even when it reports no update available.
func (s *Service) upgradeAgentInternal(ctx context.Context, env *environment.EnvironmentService, envID string, job *EnvironmentUpdateJob, result *EnvironmentUpdateResult) {
	defer result.clearStageInternal()
	s.setUpdateStageInternal(ctx, job, result, EnvironmentUpdateStageChecking)
	versionCtx, cancel := context.WithTimeout(ctx, updateAllAgentRequestTimeoutInternal)
	var info versiontypes.Info
	err := env.ProxyJSONRequest(versionCtx, envID, http.MethodGet, "/api/app-version", nil, &info)
	cancel()
	if err != nil {
		result.Status = updateAllAgentFailureStatusInternal(err)
		result.Error = truncateUpdateAllErrorInternal(err)
		return
	}
	// Validate the agent observation first; then resolve an observed published
	// release under the actual hub ceiling when latest is too new.
	target := info.NewestVersion
	targetErr := imageDTO.ValidateAgentUpgradeTarget(target, config.Version)
	if targetErr != nil && imageDTO.Neutral(targetErr, "update.system-version-unknown", 503).Problem.Code == "update.requires-hub-update" {
		target, targetErr = s.versionService.PublishedReleaseAtMost(ctx, config.Version)
	}
	if targetErr != nil {
		result.Status = EnvironmentUpdateResultStatusFailed
		result.Error = imageDTO.Neutral(targetErr, "update.system-version-unknown", 503).Problem.Code
		return
	}

	if compared, ok := imageDTO.CompareRelease(target, info.CurrentVersion); !ok || compared < 0 {
		result.Status = EnvironmentUpdateResultStatusFailed
		result.Error = "update.invalid-target"
		return
	}
	if target != info.NewestVersion {
		info.NewestVersion = target
		info.NewestDigest = ""
		info.UpdateAvailable = strings.TrimPrefix(target, "v") != strings.TrimPrefix(info.CurrentVersion, "v")
	}
	triggerBody, err := json.Marshal(TriggerUpgradeBody{TargetVersion: target})
	if err != nil {
		result.Status = EnvironmentUpdateResultStatusFailed
		result.Error = "update.invalid-target"
		return
	}

	result.FromVersion = info.CurrentVersion
	result.ToVersion = updateAllTargetVersionInternal(&info)

	s.setUpdateStageInternal(ctx, job, result, EnvironmentUpdateStageStarting)
	triggerCtx, cancel := context.WithTimeout(ctx, updateAllAgentRequestTimeoutInternal)
	resp, err := env.ExecuteRemoteRequest(triggerCtx, envID, http.MethodPost, "/api/environments/0/system/upgrade", triggerBody)
	cancel()
	if err != nil {
		result.Status = EnvironmentUpdateResultStatusFailed
		result.Error = truncateUpdateAllErrorInternal(err)
		return
	}
	if requireSuccessErr := resp.RequireSuccess(); requireSuccessErr != nil {
		result.Status = EnvironmentUpdateResultStatusFailed
		result.Error = truncateUpdateAllErrorInternal(requireSuccessErr)
		return
	}

	// An agent that reports no update available and already runs the target has
	// nothing to swap in: its upgrader pulls, finds the same image and skips the
	// recreate. Confirming that would only burn the poll window waiting for a version
	// change that cannot come, so record it directly.
	if info.AlreadyOnNewest() {
		result.Status = EnvironmentUpdateResultStatusUpToDate
		result.ToVersion = info.CurrentVersion
		return
	}

	s.setUpdateStageInternal(ctx, job, result, EnvironmentUpdateStageReconnecting)
	if s.confirmAgentUpgradedInternal(ctx, env, envID, info, job, result) {
		result.Status = EnvironmentUpdateResultStatusUpdated
	} else {
		// Upgrade fired but the new version was not confirmed within the wait window.
		result.Status = EnvironmentUpdateResultStatusTriggered
	}
}

// updateAllAgentFailureStatusInternal classifies a failed agent pre-check. An
// environment we actually reached but whose request failed or timed out is a real
// failure, not an offline skip: poll-mode agents connect on demand, so a slow
// tunnel round-trip surfaces here as a deadline even though the agent is online.
// Only errors that look like the environment was never reachable (no tunnel,
// connection refused, DNS failure, …) stay an offline skip.
func updateAllAgentFailureStatusInternal(err error) EnvironmentUpdateResultStatus {
	// The tunnel/connection was established but the request did not finish: it
	// either timed out or was canceled (e.g. the parent context was aborted).
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return EnvironmentUpdateResultStatusFailed
	}
	// The environment answered with a non-success status — reached, not offline.
	if _, ok := errors.AsType[*remenv.StatusError](err); ok {
		return EnvironmentUpdateResultStatusFailed
	}
	return EnvironmentUpdateResultStatusSkippedOffline
}

// confirmAgentUpgradedInternal polls the agent's version until it moves off the
// pre-upgrade baseline or reports the upgrade target, or the wait window elapses.
// The target is the same fallback-resolved identifier recorded in ToVersion, so a
// force-update of an already-latest agent — including one whose version check
// could not determine the latest release — confirms on a same-image recreation
// instead of timing out to triggered.
func (s *Service) confirmAgentUpgradedInternal(
	ctx context.Context, env *environment.EnvironmentService, envID string,
	baseline versiontypes.Info, job *EnvironmentUpdateJob, result *EnvironmentUpdateResult,
) bool {
	target := updateAllTargetVersionInternal(&baseline)
	deadline := time.Now().Add(updateAllConfirmTimeoutInternal)
	ticker := time.NewTicker(updateAllConfirmPollIntervalInternal)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(ctx, updateAllAgentRequestTimeoutInternal)
			var info versiontypes.Info
			err := env.ProxyJSONRequest(reqCtx, envID, http.MethodGet, "/api/app-version", nil, &info)
			cancel()
			if err == nil {
				s.setUpdateStageInternal(ctx, job, result, EnvironmentUpdateStageVerifying)
				versionChanged := baseline.CurrentVersion != "" && info.CurrentVersion != baseline.CurrentVersion
				digestChanged := baseline.CurrentDigest != "" && info.CurrentDigest != baseline.CurrentDigest
				onTarget := target != "" &&
					(strings.TrimPrefix(info.CurrentVersion, "v") == strings.TrimPrefix(target, "v") ||
						info.CurrentDigest == target)
				if versionChanged || digestChanged || onTarget {
					return true
				}
			} else {
				s.setUpdateStageInternal(ctx, job, result, EnvironmentUpdateStageReconnecting)
			}
			if time.Now().After(deadline) {
				return false
			}
		}
	}
}

// setUpdateStageInternal records which step result is in and persists the job so
// the status endpoint reflects it; re-entering the current stage keeps its start time.
func (s *Service) setUpdateStageInternal(ctx context.Context, job *EnvironmentUpdateJob, result *EnvironmentUpdateResult, stage EnvironmentUpdateStage) {
	if result.Stage == stage {
		return
	}
	result.Stage = stage
	result.StageStartedAt = new(time.Now())
	if err := s.persistUpdateAllJobInternal(ctx, job); err != nil {
		slog.WarnContext(ctx, "update-all: failed to persist stage", "jobId", job.ID, "environmentId", result.EnvironmentID, "stage", stage, "error", err)
	}
}

// GetLatestUpdateAllJob returns the most recently created update-all job, or nil.
func (s *Service) GetLatestUpdateAllJob(ctx context.Context) (*EnvironmentUpdateJob, error) {
	var jobs []EnvironmentUpdateJob
	if err := s.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(1).
		Find(&jobs).Error; err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return &jobs[0], nil
}

func (s *Service) activeUpdateAllJobInternal(ctx context.Context) (*EnvironmentUpdateJob, error) {
	var jobs []EnvironmentUpdateJob
	if err := s.db.WithContext(ctx).
		Where("status IN ?", []string{
			string(EnvironmentUpdateJobStatusPendingRestart),
			string(EnvironmentUpdateJobStatusRunning),
		}).
		Order("created_at DESC").
		Limit(1).
		Find(&jobs).Error; err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return &jobs[0], nil
}

func (s *Service) getUpdateAllJobByIDInternal(ctx context.Context, id string) (*EnvironmentUpdateJob, error) {
	var jobs []EnvironmentUpdateJob
	if err := s.db.WithContext(ctx).
		Where("id = ?", id).
		Limit(1).
		Find(&jobs).Error; err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return &jobs[0], nil
}

func (s *Service) persistUpdateAllJobInternal(ctx context.Context, job *EnvironmentUpdateJob) error {
	return s.db.WithContext(ctx).Save(job).Error
}

func (s *Service) markUpdateAllFailedInternal(ctx context.Context, job *EnvironmentUpdateJob, reason string) {
	job.Status = EnvironmentUpdateJobStatusFailed
	job.Error = &reason
	job.CompletedAt = new(time.Now())
	for i := range job.Results {
		job.Results[i].clearStageInternal()
		if job.Results[i].Status == EnvironmentUpdateResultStatusUpdating {
			job.Results[i].Status = EnvironmentUpdateResultStatusFailed
			job.Results[i].Error = reason
		}
	}
	if err := s.persistUpdateAllJobInternal(ctx, job); err != nil {
		slog.WarnContext(ctx, "update-all: failed to mark job failed", "jobId", job.ID, "error", err)
	}

	// Surface the failure in the events audit log; the success path logs via
	// logUpdateAllEventInternal. LogUserEvent hardcodes an info-severity "completed"
	// title, so create the event directly with error severity and the reason.
	if _, err := s.eventService.CreateEvent(ctx, event.CreateEventRequest{
		Type:        event.EventTypeSystemUpgrade,
		Severity:    event.EventSeverityError,
		Title:       "Update all environments failed",
		Description: reason,
		UserID:      new(job.UserID),
		Username:    new(job.Username),
		Metadata: database.JSON{
			"action":       "update_all_environments",
			"jobId":        job.ID,
			"environments": len(job.Results),
			"reason":       reason,
		},
	}); err != nil {
		slog.WarnContext(ctx, "update-all: failed to log failure event", "jobId", job.ID, "error", err)
	}

	slog.WarnContext(ctx, "Update-all job failed", "jobId", job.ID, "reason", reason)
}

// recordManagerResultInternal sets the manager (env "0") entry to its final status:
// updated after a confirmed restart, up_to_date when the pull found nothing to swap
// in, or failed otherwise.
func (s *Service) recordManagerResultInternal(job *EnvironmentUpdateJob, status EnvironmentUpdateResultStatus, currentVersion string) {
	manager := managerResultInternal(job)
	if manager == nil {
		return
	}
	manager.Status = status
	switch status {
	case EnvironmentUpdateResultStatusUpdated, EnvironmentUpdateResultStatusUpToDate:
		manager.ToVersion = currentVersion
	case EnvironmentUpdateResultStatusFailed:
		manager.Error = "manager version did not change after upgrade"
	case EnvironmentUpdateResultStatusPending,
		EnvironmentUpdateResultStatusUpdating,
		EnvironmentUpdateResultStatusTriggered,
		EnvironmentUpdateResultStatusSkippedOffline:
		// Nothing more to record: the row keeps the target version it was seeded
		// with, since none of these outcomes establishes what it ended up running.
	}
}

// finalizeUpdateAllJobInternal closes a job out as completed and records the audit
// event. The per-environment rows must already carry their final statuses.
func (s *Service) finalizeUpdateAllJobInternal(ctx context.Context, job *EnvironmentUpdateJob) {
	job.Status = EnvironmentUpdateJobStatusCompleted
	job.CompletedAt = new(time.Now())
	for i := range job.Results {
		job.Results[i].clearStageInternal()
	}
	if err := s.persistUpdateAllJobInternal(ctx, job); err != nil {
		slog.WarnContext(ctx, "update-all: failed to finalize job", "jobId", job.ID, "error", err)
		return
	}
	s.logUpdateAllEventInternal(ctx, job)
}

func (s *Service) logUpdateAllEventInternal(ctx context.Context, job *EnvironmentUpdateJob) {
	failed := 0
	for _, r := range job.Results {
		if r.Status == EnvironmentUpdateResultStatusFailed {
			failed++
		}
	}

	metadata := database.JSON{
		"action":       "update_all_environments",
		"jobId":        job.ID,
		"environments": len(job.Results),
		"failed":       failed,
	}

	// All environments succeeded: log the standard completed (info) event.
	if failed == 0 {
		if err := s.eventService.LogUserEvent(ctx, event.EventTypeSystemUpgrade, job.UserID, job.Username, metadata); err != nil {
			slog.WarnContext(ctx, "Failed to log update-all event", "jobId", job.ID, "error", err)
		}
		return
	}

	// The job ran to completion but some environments failed to update — record a
	// warning-severity event so those failures still show in the audit log.
	if _, err := s.eventService.CreateEvent(ctx, event.CreateEventRequest{
		Type:        event.EventTypeSystemUpgrade,
		Severity:    event.EventSeverityWarning,
		Title:       "Update all environments completed with errors",
		Description: fmt.Sprintf("%d of %d environments failed to update", failed, len(job.Results)),
		UserID:      new(job.UserID),
		Username:    new(job.Username),
		Metadata:    metadata,
	}); err != nil {
		slog.WarnContext(ctx, "Failed to log update-all event", "jobId", job.ID, "error", err)
	}
}

// updateAllTargetVersionInternal picks the best human-readable target identifier:
// the newest version tag if known, otherwise the newest digest. When the version
// check could not determine the latest release (offline, rate-limited) it falls
// back to the current identifiers: updates run unconditionally, so the target of a
// force-update with an unknown latest is wherever the pull lands — recording the
// current version keeps the resume check able to recognize a same-image recreation
// as success instead of finalizing it as failed.
func updateAllTargetVersionInternal(info *versiontypes.Info) string {
	if info == nil {
		return ""
	}
	if info.NewestVersion != "" {
		return info.NewestVersion
	}
	if info.NewestDigest != "" {
		return info.NewestDigest
	}
	if info.CurrentVersion != "" {
		return info.CurrentVersion
	}
	return info.CurrentDigest
}

func truncateUpdateAllErrorInternal(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > updateAllErrorMaxLenInternal {
		return msg[:updateAllErrorMaxLenInternal]
	}
	return msg
}

// PruneUpgradeLogs removes expired upgrade logs from this instance's data directory.
func (s *Service) PruneUpgradeLogs(ctx context.Context, dataDir string, now time.Time) (int, error) {
	retentionDays := s.settingsService.GetIntSetting(ctx, "upgradeLogRetentionDays", 3)
	if retentionDays == 0 {
		return 0, nil
	}
	if retentionDays < 0 || retentionDays > 3650 {
		return 0, errors.New("upgrade log retention must be between 0 and 3650 days")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	root, err := os.OpenRoot(dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open upgrade log directory: %w", err)
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			slog.WarnContext(ctx, "Failed to close upgrade log directory", "error", closeErr)
		}
	}()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return 0, fmt.Errorf("read upgrade log directory: %w", err)
	}

	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	removed := 0
	var failures []error
	for _, entry := range entries {
		if cancellationErr := ctx.Err(); cancellationErr != nil {
			return removed, errors.Join(append(failures, cancellationErr)...)
		}
		if !upgradeLogNameInternal.MatchString(entry.Name()) {
			continue
		}
		info, lstatErr := root.Lstat(entry.Name())
		if errors.Is(lstatErr, os.ErrNotExist) {
			continue
		}
		if lstatErr != nil {
			failures = append(failures, fmt.Errorf("inspect upgrade log %s: %w", entry.Name(), lstatErr))
			continue
		}
		if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
			continue
		}
		if removeErr := root.Remove(entry.Name()); removeErr != nil {
			if !errors.Is(removeErr, os.ErrNotExist) {
				failures = append(failures, fmt.Errorf("remove upgrade log %s: %w", entry.Name(), removeErr))
			}
			continue
		}
		removed++
	}
	return removed, errors.Join(failures...)
}
