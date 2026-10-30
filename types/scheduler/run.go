// Source: getarcaneapp/arcane/types/scheduler/run.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package scheduler

import (
	"context"
	"encoding/json/jsontext"
	"github.com/florianfinn/docklet-next/types/image"
	"time"
)

// RunStatus describes durable execution, independently of the configured schedule.
type RunStatus string

const (
	Queued    RunStatus = "queued"
	Waiting   RunStatus = "waiting"
	Running   RunStatus = "running"
	Retrying  RunStatus = "retrying"
	Succeeded RunStatus = "succeeded"
	Partial   RunStatus = "partial"
	Skipped   RunStatus = "skipped"
	Failed    RunStatus = "failed"
	// NeedsAttention preserves domain recovery evidence and legacy job outcomes.
	NeedsAttention RunStatus = "needs_attention"
	Canceled       RunStatus = "canceled"
)

func (s RunStatus) Terminal() bool {
	return s == Succeeded || s == Partial || s == Skipped || s == Failed || s == Canceled
}

type TargetOutcome struct {
	Code         image.ReasonCode `json:"code,omitempty" enum:"autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,feature-disabled,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.response-too-large,vulnerability.request-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,vulnerability.storage-unavailable"`
	RecoveryData jsontext.Value   `json:"-" msgpack:"recoveryData,omitempty"`
	ResourceType string           `json:"resourceType,omitempty"`
	ID           string           `json:"id"`
	Status       RunStatus        `json:"status"`
	Message      string           `json:"message,omitempty"`
	ActivityID   string           `json:"activityId,omitempty"`
}

type Outcome struct {
	Code       image.ReasonCode `json:"code,omitempty" enum:"autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,feature-disabled,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.response-too-large,vulnerability.request-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,vulnerability.storage-unavailable"`
	Status     RunStatus        `json:"status"`
	Message    string           `json:"message,omitempty"`
	ActivityID string           `json:"activityId,omitempty"`
	Targets    []TargetOutcome  `json:"targets,omitempty"`
}

type Request struct {
	// ObservedAgentRun adopts existing agent work without another submission.
	// TODO(v3): remove legacy-agent adoption.
	ObservedAgentRun *Run   `json:"-"`
	RunID            string `json:"runId,omitempty"`
	JobID            string `json:"jobId"`
	EnvironmentID    string `json:"environmentId"`
	Trigger          string `json:"trigger"`
	RequestedWithKey string `json:"requestedWithKey,omitempty"`
	RequestedBy      string `json:"requestedBy,omitempty"`
}

type Attempt struct {
	Number     int        `json:"number"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Outcome    Outcome    `json:"outcome"`
}

// RunResolution preserves legacy resolution history.
// TODO(v3): remove this deprecated mixed-version compatibility contract.
type RunResolution struct {
	ResolvedBy string    `json:"resolvedBy"`
	ResolvedAt time.Time `json:"resolvedAt"`
	Reason     string    `json:"reason"`
}

type Run struct {
	ActivityEnvironmentID   string         `json:"activityEnvironmentId,omitempty"`
	ActivityID              string         `json:"activityId,omitempty"`
	Resolution              *RunResolution `json:"resolution,omitempty"`
	RequestedWithKey        string         `json:"requestedWithKey,omitempty"`
	ID                      string         `json:"id"`
	JobID                   string         `json:"jobId"`
	EnvironmentID           string         `json:"environmentId"`
	Trigger                 string         `json:"trigger"`
	RequestedBy             string         `json:"requestedBy,omitempty"`
	Status                  RunStatus      `json:"status"`
	CreatedAt               time.Time      `json:"createdAt"`
	UpdatedAt               time.Time      `json:"updatedAt"`
	StartedAt               *time.Time     `json:"startedAt,omitempty"`
	FinishedAt              *time.Time     `json:"finishedAt,omitempty"`
	NextAttempt             *time.Time     `json:"nextAttempt,omitempty"`
	AttemptCount            int            `json:"attemptCount"`
	Owner                   string         `json:"owner,omitempty"`
	Outcome                 Outcome        `json:"outcome"`
	Attempts                []Attempt      `json:"attempts,omitempty"`
	RemoteDeliveryAttempted bool           `json:"remoteDeliveryAttempted"`
	RemoteOutcome           *Outcome       `json:"remoteOutcome,omitempty"`
	RemoteRetryRequested    bool           `json:"remoteRetryRequested,omitempty"`
	RemoteRetryAttempted    bool           `json:"remoteRetryAttempted,omitempty"`
	RemoteAttemptCount      int            `json:"remoteAttemptCount,omitempty"`
	RemoteAccepted          bool           `json:"remoteAccepted"`
	RemoteSettled           bool           `json:"remoteSettled"`
	LastConfirmedAt         *time.Time     `json:"lastConfirmedAt,omitempty"`
}

type RunList struct {
	Runs  []Run `json:"runs"`
	Total int   `json:"total"`
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
}

type WorkerHealth struct {
	Status    string     `json:"status"`
	LastError string     `json:"lastError,omitempty"`
	NextRetry *time.Time `json:"nextRetry,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Reconciler consults domain state before repeating interrupted work.
type Reconciler interface {
	Reconcile(ctx context.Context, previous Run) (Outcome, error)
}

// OutcomeError carries partial results through error-only watcher contracts.
type OutcomeError struct {
	Outcome Outcome
	Cause   error
}

func (e *OutcomeError) Error() string {
	if e.Outcome.Message != "" {
		return e.Outcome.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Outcome.Status)
}

func (e *OutcomeError) Unwrap() error { return e.Cause }

// RetryValidator rejects retries without safe persisted target evidence.
type RetryValidator interface {
	ValidateRetry(ctx context.Context, run Run) error
}
