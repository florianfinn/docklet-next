// Source: docklet-next/docs/design/update-policies.md
// Revision: 4023414fcf620a9ea4be2429ee70fa8fd8230f9c
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Shared image, runtime and policy reason vocabulary with compatible string values.

package image

import (
	"errors"
	"strings"
)

// ReasonCode keeps existing string callers compatible. Every public reason field
// exposes ReasonEnum; the contract guard rejects drift between those declarations.
type ReasonCode = string

const ReasonEnum = "autoheal.interrupted,autoheal.limit-reached,autoheal.restart-failed,autoheal.restarted,feature-disabled,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.response-too-large,vulnerability.request-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,vulnerability.storage-unavailable"

func ValidReason(code string) bool {
	for _, declared := range strings.Split(ReasonEnum, ",") {
		if declared == code {
			return true
		}
	}
	return false
}

// ErrUndeclaredReason rejects values outside the shared wire vocabulary.
var ErrUndeclaredReason = errors.New("undeclared reason code")

// ValidateReason accepts an omitted optional reason or a declared enum value.
func ValidateReason(code string) error {
	if code != "" && !ValidReason(code) {
		return ErrUndeclaredReason
	}
	return nil
}
