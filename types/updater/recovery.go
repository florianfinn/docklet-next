// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed update recovery contracts.

package updater

import (
	"context"
	"slices"
)

const MaxSuppressedCandidates = 64
const MaxAppliedCandidates = 256
// Evidence holds up to MaxAppliedCandidates applied tuples of realistic length.
const MaxRecoveryEvidenceBytes = 64 << 10

// Candidate identities use pullable platform manifests, never native image IDs.
type AppliedCandidate struct {
	Service    string `json:"service" maxLength:"256"`
	Repository string `json:"repository" maxLength:"1024"`
	Platform   string `json:"platform" maxLength:"256"`
	Manifest   string `json:"manifest" pattern:"^sha256:[a-f0-9]{64}$"`
}
type SuppressedCandidate = AppliedCandidate

type RecoveryResult struct {
	Outcome string `json:"outcome" enum:"restored,restore-failed,unavailable,not-needed,interrupted"`
	Code    string `json:"code,omitempty" enum:"agent.unavailable,container.origin-locked,container.recreate-ambiguous,container.recreate-recovered,autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,backup.copy.insufficient-space,backup.copy.mount-failed,backup.copy.write-failed,deploy.point-unavailable,feature-disabled,history.invalid-range,history.sample-invalid,history.sample-too-large,history.unavailable,host.restarting,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,preferences.invalid,preferences.no-user,preferences.too-large,preferences.unknown-section,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,settings.invalid-time-zone,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.service-coupled,update.snapshot-unavailable,update.system-version-unknown,update.candidate-changed,update.target-changed,update.target-not-found,volume.workspace-busy,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.request-too-large,vulnerability.response-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,candidate.deferred,candidate.unavailable,vulnerability.storage-unavailable,update.unstable,update.auto-restored,update.auto-restore-failed,update.auto-restore-unavailable,update.interrupted,update.candidate-failed,restore.files-changed,container.auto-remove"`
}

// ObservedExecution is installed by runtime dispatch after canonical admission.
// An empty Mode means the ordinary manual path with no observation or recovery.
type ObservedExecution struct {
	Mode, PolicyID, ChildID string
	Suppressed              []SuppressedCandidate
}
type observedExecutionKey struct{}

func WithObservedExecution(ctx context.Context, value ObservedExecution) context.Context {
	value.Suppressed = slices.Clone(value.Suppressed)
	return context.WithValue(ctx, observedExecutionKey{}, value)
}
func ObservedExecutionFromContext(ctx context.Context) ObservedExecution {
	value, _ := ctx.Value(observedExecutionKey{}).(ObservedExecution)
	value.Suppressed = slices.Clone(value.Suppressed)
	return value
}
