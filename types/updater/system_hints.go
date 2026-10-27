// Source: docklet-next/docs/design/images-updates.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Informational agent/tools references with a hub-computed stable release ceiling.

package updater

import "github.com/florianfinn/docklet-next/types/image"

type SystemImageHint struct {
	Kind                string           `json:"kind" enum:"agent,tools"`
	Reference           string           `json:"reference"`
	CurrentVersion      string           `json:"currentVersion,omitempty"`
	CandidateVersion    string           `json:"candidateVersion,omitempty"`
	CompatibleReference string           `json:"compatibleReference,omitempty"`
	State               string           `json:"state" enum:"current,available,pinned,unknown"`
	Code                image.ReasonCode `json:"code,omitempty" enum:"feature-disabled,image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,job.skipped,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,run.busy,run.cancelling,runtime.interrupted,runtime.outcome-unknown,runtime.parent-unresolved,runtime.target-changed,runtime.target-unavailable,update.agent-local-job-disabled,update.auto-excluded,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-policy,update.invalid-target,update.path-retired,update.permission-denied,update.persistence-failed,update.policy-changed,update.requires-hub-update,update.schedule-hub-owned,update.schedule-owner-unset,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found,vulnerability.agent-job-retired,vulnerability.busy,vulnerability.database-unavailable,vulnerability.dispatch-interrupted,vulnerability.dispatch-unknown,vulnerability.host-incompatible,vulnerability.host-offline,vulnerability.host-pending,vulnerability.image-not-found,vulnerability.image-removed,vulnerability.no-eligible-hosts,vulnerability.platform-unavailable,vulnerability.report-invalid,vulnerability.response-too-large,vulnerability.request-too-large,vulnerability.run-not-found,vulnerability.scan-cancelled,vulnerability.scan-failed,vulnerability.scan-interrupted,vulnerability.scan-timeout,vulnerability.scanner-unavailable,vulnerability.storage-full,vulnerability.storage-unavailable"`
	RequiresHubUpdate   bool             `json:"requiresHubUpdate"`
	ManualOnly          bool             `json:"manualOnly"`
}
type SystemImageHints struct {
	HubVersion string            `json:"hubVersion"`
	Images     []SystemImageHint `json:"images"`
}
