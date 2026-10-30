// Source: getarcaneapp/arcane/types/jobschedule/jobschedule.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package jobschedule

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/types/image"
	"time"

	"github.com/florianfinn/docklet-next/types/scheduler"
)

// Config contains cron schedules for Arcane background jobs.
type Config struct {
	EnvironmentHealthInterval      string `json:"environmentHealthInterval"`
	EventCleanupInterval           string `json:"eventCleanupInterval"`
	ExpiredSessionsCleanupInterval string `json:"expiredSessionsCleanupInterval"`
	AutoUpdateInterval             string `json:"-"`
	DockerClientRefreshInterval    string `json:"dockerClientRefreshInterval"`
	PollingInterval                string `json:"-"`
	ScheduledPruneInterval         string `json:"scheduledPruneInterval"`
	VulnerabilityScanInterval      string `json:"vulnerabilityScanInterval"`
	ImageAutoPatchInterval         string `json:"imageAutoPatchInterval"`
	AutoHealInterval               string `json:"autoHealInterval"`
}

// Update changes job cron schedules.
//
// Any nil field is ignored.
type Update struct {
	HubOwnedFields                 bool    `json:"-"`
	EnvironmentHealthInterval      *string `json:"environmentHealthInterval,omitzero"`
	EventCleanupInterval           *string `json:"eventCleanupInterval,omitzero"`
	ExpiredSessionsCleanupInterval *string `json:"expiredSessionsCleanupInterval,omitzero"`
	AutoUpdateInterval             any     `json:"autoUpdateInterval,omitzero" hidden:"true"`
	DockerClientRefreshInterval    *string `json:"dockerClientRefreshInterval,omitzero"`
	PollingInterval                any     `json:"pollingInterval,omitzero" hidden:"true"`
	ScheduledPruneInterval         *string `json:"scheduledPruneInterval,omitzero"`
	VulnerabilityScanInterval      *string `json:"vulnerabilityScanInterval,omitzero"`
	ImageAutoPatchInterval         *string `json:"imageAutoPatchInterval,omitzero"`
	AutoHealInterval               *string `json:"autoHealInterval,omitzero"`
}

// JobStatus represents the current status and metadata for a background job.
type JobStatus struct {
	HostID         string                  `json:"hostId,omitempty"`
	Kind           string                  `json:"kind,omitempty"`
	TimeZone       string                  `json:"timeZone,omitempty"`
	Code           image.ReasonCode        `json:"code,omitempty" enum:"autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,feature-disabled,host.restarting,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.response-too-large,vulnerability.request-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,vulnerability.storage-unavailable"`
	CurrentRun     *scheduler.Run          `json:"currentRun,omitempty"`
	LastRun        *scheduler.Run          `json:"lastRun,omitempty"`
	LastSuccess    *time.Time              `json:"lastSuccess,omitempty"`
	LastError      string                  `json:"lastError,omitempty"`
	WorkerHealth   *scheduler.WorkerHealth `json:"workerHealth,omitempty"`
	Children       []JobStatus             `json:"children,omitempty"`
	ID             string                  `json:"id"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	Category       string                  `json:"category"`
	Schedule       string                  `json:"schedule"`
	NextRun        *time.Time              `json:"nextRun,omitempty"`
	Enabled        bool                    `json:"enabled"`
	ManagerOnly    bool                    `json:"managerOnly"`
	IsContinuous   bool                    `json:"isContinuous"`
	CanRunManually bool                    `json:"canRunManually"`
	Prerequisites  []JobPrerequisite       `json:"prerequisites"`
	SettingsKey    string                  `json:"settingsKey,omitempty"`
}

// JobPrerequisite represents a requirement that must be met for a job to run.
type JobPrerequisite struct {
	SettingKey  string `json:"settingKey"`
	Label       string `json:"label"`
	IsMet       bool   `json:"isMet"`
	SettingsURL string `json:"settingsUrl,omitempty"`
}

// JobListResponse contains all jobs and system mode information.
type JobListResponse struct {
	DurableRuns bool        `json:"durableRuns"`
	ObservedAt  time.Time   `json:"observedAt"`
	Offline     bool        `json:"offline,omitempty"`
	Jobs        []JobStatus `json:"jobs"`
	IsAgent     bool        `json:"isAgent"`
}

// JobRunRequest is the request to manually run a job.
type JobRunRequest struct {
	JobID string `json:"jobId" path:"jobId" minLength:"1"`
}

// JobRunResponse is the response after manually triggering a job.
type JobRunResponse struct {
	RunID   string              `json:"runId,omitempty"`
	Status  scheduler.RunStatus `json:"status,omitempty"`
	Success bool                `json:"success"`
	Message string              `json:"message"`
}

// SubmitRunInput preserves identity across manager-to-agent delivery retries.
type SubmitRunInput struct {
	RunID string `json:"runId"`
}

func (u *Update) UnmarshalJSON(body []byte) error {
	type plain Update
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	*u = Update(decoded)
	_, auto := raw["autoUpdateInterval"]
	_, polling := raw["pollingInterval"]
	u.HubOwnedFields = auto || polling
	return nil
}
func (u Update) HasHubOwnedFields() bool {
	return u.HubOwnedFields || u.AutoUpdateInterval != nil || u.PollingInterval != nil
}
