// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Stack-only update and restore intent; snapshots never cross this boundary. M21 #438: candidate scans and verified manifest binding. M24 recovery evidence, validation and reason contracts.

package updater

import (
	"context"
	"time"
)

// Complete restore evidence must fit the same frozen intent limits as admission.
const MaxLifecycleServices = 256
const MaxLifecycleIntentBytes = 128 << 10

type LifecycleRequest struct {
	AutoRestore          bool     `json:"autoRestore,omitempty"`
	Service              string   `json:"service,omitempty" maxLength:"256"`
	ExpectedManifest     string   `json:"expectedManifest,omitempty" pattern:"^sha256:[a-f0-9]{64}$"`
	Services             []string `json:"services,omitempty" maxItems:"256"`
	PointID              string   `json:"pointId,omitempty"`
	ExpectedRevision     string   `json:"expectedRevision" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
	ReplaceConfiguration bool     `json:"replaceConfiguration,omitempty"`
}

// LifecycleIntent is produced by executor preflight, then hashed into runtime
// admission. It contains identities only, never Compose/environment contents.
type LifecycleIntent struct {
	Policy           *PolicyBinding   `json:"policy,omitempty"`
	Request          LifecycleRequest `json:"request"`
	TargetIdentity   string           `json:"targetIdentity"`
	SavedRevision    string           `json:"savedRevision"`
	ObservedRevision string           `json:"observedRevision"`
	BaselineRevision string           `json:"baselineRevision"`
	PointRevision    string           `json:"pointRevision"`
	Selections       []ImageSelection `json:"selections" maxItems:"256"`
}

type ImageSelection struct {
	Service         string `json:"service"`
	SourceReference string `json:"sourceReference"`
	Reference       string `json:"reference"`
	Digest          string `json:"digest"`
	ImageID         string `json:"imageId"`
	Platform        string `json:"platform"`
	Kind            string `json:"kind" enum:"version,rebuild,restore"`
}

type PointImage struct {
	Service   string `json:"service"`
	Reference string `json:"reference"`
	ImageID   string `json:"imageId"`
	Digest    string `json:"digest,omitempty"`
	Present   bool   `json:"present"`
	Running   bool   `json:"running"`
}
type RestorePoint struct {
	ProtectionReason   *RestorePointProtectionReason `json:"protectionReason" nullable:"true" enum:"restore-target,healthy-baseline"`
	HoldState          RestorePointHoldState         `json:"holdState" enum:"none,active,unknown"`
	FailedAttempt      bool                          `json:"failedAttempt"`
	Images             []PointImage                  `json:"images" nullable:"false"`
	ID                 string                        `json:"id"`
	RegistrationID     string                        `json:"registrationId"`
	TargetID           string                        `json:"targetId"`
	TargetIdentity     string                        `json:"targetIdentity"`
	SourceRunID        string                        `json:"sourceRunId"`
	CreatedAt          time.Time                     `json:"createdAt"`
	SourceState        string                        `json:"sourceState" enum:"settled,partial-source"`
	VolumeDataIncluded bool                          `json:"volumeDataIncluded"`
	Available          bool                          `json:"available"`
	Orphaned           bool                          `json:"orphaned"`
	Code               string                        `json:"code,omitempty" enum:"agent.unavailable,container.origin-locked,container.recreate-ambiguous,container.recreate-recovered,autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,backup.copy.insufficient-space,backup.copy.mount-failed,backup.copy.write-failed,deploy.point-unavailable,feature-disabled,history.invalid-range,history.sample-invalid,history.sample-too-large,history.unavailable,host.restarting,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,preferences.invalid,preferences.no-user,preferences.too-large,preferences.unknown-section,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,settings.invalid-time-zone,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.service-coupled,update.snapshot-unavailable,update.system-version-unknown,update.candidate-changed,update.target-changed,update.target-not-found,volume.workspace-busy,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.request-too-large,vulnerability.response-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,candidate.deferred,candidate.unavailable,vulnerability.storage-unavailable,update.unstable,update.auto-restored,update.auto-restore-failed,update.auto-restore-unavailable,update.interrupted,update.candidate-failed,restore.files-changed,container.auto-remove"`
}

type BaselineStatus struct {
	TargetIdentity   string `json:"targetIdentity"`
	AutoExcluded     bool   `json:"autoExcluded"`
	SavedRevision    string `json:"savedRevision"`
	ObservedRevision string `json:"observedRevision"`
	Available        bool   `json:"available"`
	Code             string `json:"code,omitempty" enum:"agent.unavailable,container.origin-locked,container.recreate-ambiguous,container.recreate-recovered,autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,backup.copy.insufficient-space,backup.copy.mount-failed,backup.copy.write-failed,deploy.point-unavailable,feature-disabled,history.invalid-range,history.sample-invalid,history.sample-too-large,history.unavailable,host.restarting,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,preferences.invalid,preferences.no-user,preferences.too-large,preferences.unknown-section,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,settings.invalid-time-zone,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.service-coupled,update.snapshot-unavailable,update.system-version-unknown,update.candidate-changed,update.target-changed,update.target-not-found,volume.workspace-busy,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.request-too-large,vulnerability.response-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,candidate.deferred,candidate.unavailable,vulnerability.storage-unavailable,update.unstable,update.auto-restored,update.auto-restore-failed,update.auto-restore-unavailable,update.interrupted,update.candidate-failed,restore.files-changed,container.auto-remove"`
	SourceRunID      string `json:"sourceRunId,omitempty"`
}

// ExecutionIdentity is installed only by the trusted runtime adapter.
type ExecutionIdentity struct{ RunID, RegistrationID string }
type executionKey struct{}

func WithExecutionIdentity(ctx context.Context, id ExecutionIdentity) context.Context {
	return context.WithValue(ctx, executionKey{}, id)
}
func ExecutionIdentityFromContext(ctx context.Context) ExecutionIdentity {
	v, _ := ctx.Value(executionKey{}).(ExecutionIdentity)
	return v
}

// LifecycleResult is bounded public run evidence; no snapshots or daemon output.
type LifecycleResult struct {
	Recovery           *RecoveryResult    `json:"recovery,omitempty"`
	Applied            []AppliedCandidate `json:"applied,omitempty" maxItems:"256"`
	HTTPStatus         int                `json:"httpStatus,omitempty"`
	RunID              string             `json:"runId"`
	RestorePointID     string             `json:"restorePointId,omitempty" maxLength:"36"`
	PointCode          string             `json:"pointCode,omitempty" enum:"agent.unavailable,container.origin-locked,container.recreate-ambiguous,container.recreate-recovered,autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,backup.copy.insufficient-space,backup.copy.mount-failed,backup.copy.write-failed,deploy.point-unavailable,feature-disabled,history.invalid-range,history.sample-invalid,history.sample-too-large,history.unavailable,host.restarting,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,preferences.invalid,preferences.no-user,preferences.too-large,preferences.unknown-section,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,settings.invalid-time-zone,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.service-coupled,update.snapshot-unavailable,update.system-version-unknown,update.candidate-changed,update.target-changed,update.target-not-found,volume.workspace-busy,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.request-too-large,vulnerability.response-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,candidate.deferred,candidate.unavailable,vulnerability.storage-unavailable,update.unstable,update.auto-restored,update.auto-restore-failed,update.auto-restore-unavailable,update.interrupted,update.candidate-failed,restore.files-changed,container.auto-remove"`
	Code               string             `json:"code,omitempty" enum:"agent.unavailable,container.origin-locked,container.recreate-ambiguous,container.recreate-recovered,autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,backup.copy.insufficient-space,backup.copy.mount-failed,backup.copy.write-failed,deploy.point-unavailable,feature-disabled,history.invalid-range,history.sample-invalid,history.sample-too-large,history.unavailable,host.restarting,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,preferences.invalid,preferences.no-user,preferences.too-large,preferences.unknown-section,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,settings.invalid-time-zone,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.service-coupled,update.snapshot-unavailable,update.system-version-unknown,update.candidate-changed,update.target-changed,update.target-not-found,volume.workspace-busy,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.request-too-large,vulnerability.response-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,candidate.deferred,candidate.unavailable,vulnerability.storage-unavailable,update.unstable,update.auto-restored,update.auto-restore-failed,update.auto-restore-unavailable,update.interrupted,update.candidate-failed,restore.files-changed,container.auto-remove"`
	BaselineCode       string             `json:"baselineCode,omitempty" enum:"agent.unavailable,container.origin-locked,container.recreate-ambiguous,container.recreate-recovered,autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,backup.copy.insufficient-space,backup.copy.mount-failed,backup.copy.write-failed,deploy.point-unavailable,feature-disabled,history.invalid-range,history.sample-invalid,history.sample-too-large,history.unavailable,host.restarting,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,preferences.invalid,preferences.no-user,preferences.too-large,preferences.unknown-section,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,settings.invalid-time-zone,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.service-coupled,update.snapshot-unavailable,update.system-version-unknown,update.candidate-changed,update.target-changed,update.target-not-found,volume.workspace-busy,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.request-too-large,vulnerability.response-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,candidate.deferred,candidate.unavailable,vulnerability.storage-unavailable,update.unstable,update.auto-restored,update.auto-restore-failed,update.auto-restore-unavailable,update.interrupted,update.candidate-failed,restore.files-changed,container.auto-remove"`
	Verified           bool               `json:"verified"`
	VolumeDataIncluded bool               `json:"volumeDataIncluded"`
}

// UncertainImageEffect retains runtime exclusion for inspected tag disposition;
// returning from a worker is not proof that an external writer can be undone.
type UncertainImageEffect struct{ Cause error }

func (e *UncertainImageEffect) Error() string          { return "runtime.outcome-unknown" }
func (e *UncertainImageEffect) RuntimeUncertain() bool { return true }

// ResultSink is an application-owned handoff to runtime settlement. A worker
// reports only bounded public evidence; the receipt transaction persists it.
type resultSinkKey struct{}

func WithResultSink(ctx context.Context, sink func(LifecycleResult)) context.Context {
	return context.WithValue(ctx, resultSinkKey{}, sink)
}
func RecordLifecycleResult(ctx context.Context, result LifecycleResult) {
	if sink, ok := ctx.Value(resultSinkKey{}).(func(LifecycleResult)); ok {
		sink(result)
	}
}
