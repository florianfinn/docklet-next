// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Required runtime projection vocabulary for activated v4 DTOs.

package activity

import "github.com/florianfinn/docklet-next/types/container"

// RuntimeOutcomeState describes receipt knowledge, independently of activity status.
// Known describes workflow disposition, not verified Docker state or rollback.
type RuntimeOutcomeState string

const (
	RuntimeOutcomeKnown   RuntimeOutcomeState = "known"
	RuntimeOutcomeUnknown RuntimeOutcomeState = "unknown"
)

// RuntimeKnowledge is required as a pair on runtime projections. Historical and
// unrelated Activity values carry neither field; cancellation intent is not outcome.
type RuntimeKnowledge struct {
	OutcomeState    RuntimeOutcomeState `json:"outcomeState" enum:"known,unknown"`
	CancelRequested bool                `json:"cancelRequested"`
}

// RuntimeSnapshot requires flat receipt knowledge beside the activity projection.
// A hub release may project failed while executor outcome remains unknown.
type RuntimeSnapshot struct {
	Result *container.RecreateResult `json:"result,omitempty"`
	Activity
	RuntimeKnowledge
}

// RuntimeAdmission confirms acceptance, never completed success. Code reuses
// CodeRunStarted or CodeRunJoined; ActivityID and Activity.ID identify the same run.
type RuntimeAdmission struct {
	ActivityID string          `json:"activityId"`
	Code       Code            `json:"code" enum:"container.recreate-recovered,container.recreate-ambiguous,container.origin-locked,runtime.idempotency-mismatch,run.request-mismatch,runtime.outcome-unknown,runtime.interrupted,runtime.cancelled,runtime.failed,runtime.wait-timeout,runtime.waiter-disconnected,runtime.parent-unresolved,runtime.target-removed,runtime.target-changed,runtime.target-unavailable,run.started,run.joined,run.fresh,run.busy,run.cancelling,activity.queued,activity.running,activity.success,activity.failed,activity.cancelled,activity.message,deploy.pull,job.queued,job.waiting,job.running,job.retrying,job.succeeded,job.partial,job.skipped,job.failed,job.needs_attention,job.canceled,container.die,container.oom,container.rename,container.unhealthy,image.untag,image.import,image.prune,volume.prune,network.prune,container.start,container.stop,container.restart,container.delete,container.create,container.scan,container.update,container.deploy,container.kill,container.pause,container.unpause,container.error,image.pull,image.load,image.tag,image.commit,image.delete,image.scan,image.error,image.vulnerability_scan,project.deploy,project.delete,project.start,project.stop,project.create,project.update,project.error,volume.create,volume.rename,volume.delete,volume.error,volume.file.create,volume.file.delete,volume.file.upload,volume.file.update,volume.workspace.update,volume.backup.create,volume.backup.delete,volume.backup.restore,volume.backup.restore_files,volume.backup.download,volume.backup.target.key_viewed,network.create,network.delete,network.connect,network.disconnect,network.error,system.prune,user.login,user.logout,system.auto_update,system.upgrade,environment.create,environment.connect,environment.disconnect,environment.update,environment.delete,environment.api_key.regenerated,environment.mtls.ca_generated,environment.mtls.cert_issued,environment.mtls.enroll,environment.mtls.download,notification.send,autoheal.restarted,autoheal.restart-failed,autoheal.limit-reached,autoheal.interrupted,notification.delivery-failed,load.sustained,container.condition.down,container.condition.unhealthy,container.condition.restart-loop,container.condition.running-again,update.self-failed,update.outcome-unknown"`
	Activity   RuntimeSnapshot `json:"activity"`
}

// Runtime codes are part of the activated v4 lifecycle vocabulary.
const (
	CodeRuntimeTargetChanged      Code = "runtime.target-changed"
	CodeRuntimeTargetUnavailable  Code = "runtime.target-unavailable"
	CodeRunRequestMismatch        Code = "run.request-mismatch"
	CodeRuntimeOutcomeUnknown     Code = "runtime.outcome-unknown"
	CodeRuntimeInterrupted        Code = "runtime.interrupted"
	CodeRuntimeCancelled          Code = "runtime.cancelled"
	CodeRuntimeFailed             Code = "runtime.failed"
	CodeRuntimeWaitTimeout        Code = "runtime.wait-timeout"
	CodeRuntimeWaiterDisconnected Code = "runtime.waiter-disconnected"
	CodeRuntimeParentUnresolved   Code = "runtime.parent-unresolved"
	CodeRuntimeTargetRemoved      Code = "runtime.target-removed"
)
