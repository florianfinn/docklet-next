// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Hub-owned policy, schedule and durable notification projections.

package updater

import (
	"github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	"time"
)

type PolicyBinding struct {
	PolicyID string `json:"policyId"`
	Revision uint64 `json:"revision"`
	JobRunID string `json:"jobRunId"`
	ChildID  string `json:"childId"`
}
type AutoUpdatePolicy struct {
	Mode            string            `json:"mode" enum:"off,notify,apply"`
	Revision        uint64            `json:"revision"`
	EnvironmentID   string            `json:"environmentId"`
	RegistrationID  string            `json:"registrationId"`
	TargetKind      string            `json:"targetKind" enum:"project"`
	TargetID        string            `json:"targetId"`
	TargetIdentity  string            `json:"targetIdentity"`
	JobID           string            `json:"jobId"`
	CheckedAt       *time.Time        `json:"checkedAt,omitempty"`
	Stale           bool              `json:"stale"`
	Code            image.ReasonCode  `json:"code,omitempty" enum:"feature-disabled,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.response-too-large,vulnerability.request-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,vulnerability.storage-unavailable"`
	ApplyCheckedAt  *time.Time        `json:"applyCheckedAt,omitempty"`
	ApplyCode       image.ReasonCode  `json:"applyCode,omitempty" enum:"feature-disabled,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.response-too-large,vulnerability.request-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,vulnerability.storage-unavailable"`
	BookkeepingAt   *time.Time        `json:"bookkeepingAt,omitempty"`
	BookkeepingCode image.ReasonCode  `json:"bookkeepingCode,omitempty" enum:"feature-disabled,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.response-too-large,vulnerability.request-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,vulnerability.storage-unavailable"`
	Candidates      []UpdateCandidate `json:"candidates"`
}
type UpdatePolicyInput struct {
	Mode             string `json:"mode" enum:"off,notify,apply"`
	ExpectedRevision uint64 `json:"expectedRevision"`
}
type UpdateCandidate struct {
	Service           string `json:"service"`
	ImageRef          string `json:"imageRef"`
	Version           string `json:"version"`
	Digest            string `json:"digest"`
	Platform          string `json:"platform"`
	EntryID           string `json:"entryId,omitempty"`
	NotificationState string `json:"notificationState" enum:"pending,sending,sent,unconfigured,failed,not-requested"`
}
type UpdateScheduleInput struct {
	ExpectedRevision   uint64 `json:"expectedRevision"`
	PollingInterval    string `json:"pollingInterval" minLength:"1" maxLength:"128"`
	AutoUpdateInterval string `json:"autoUpdateInterval" minLength:"1" maxLength:"128"`
	PollingEnabled     bool   `json:"pollingEnabled"`
	AutoUpdateEnabled  bool   `json:"autoUpdateEnabled"`
}
type UpdateScheduleConfig struct {
	Revision                uint64                  `json:"revision"`
	EnvironmentID           string                  `json:"environmentId"`
	RegistrationID          string                  `json:"registrationId"`
	PollingInterval         string                  `json:"pollingInterval"`
	AutoUpdateInterval      string                  `json:"autoUpdateInterval"`
	PollingEnabled          bool                    `json:"pollingEnabled"`
	AutoUpdateEnabled       bool                    `json:"autoUpdateEnabled"`
	MasterPollingEnabled    bool                    `json:"masterPollingEnabled"`
	MasterAutoUpdateEnabled bool                    `json:"masterAutoUpdateEnabled"`
	TimeZone                string                  `json:"timeZone"`
	Jobs                    []jobschedule.JobStatus `json:"jobs"`
}
