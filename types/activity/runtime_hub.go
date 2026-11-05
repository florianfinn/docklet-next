// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Hub recovery evidence remains separate from executor disposition.

package activity

import "time"

type RuntimeRecoveryEvidence struct {
	UnknownReason       *string    `json:"unknownReason,omitempty"`
	ExecutorState       string     `json:"executorState"`
	ExecutorResultCode  *string    `json:"executorResultCode,omitempty" enum:"container.recreate-recovered,container.recreate-ambiguous,container.origin-locked,runtime.idempotency-mismatch,run.request-mismatch,runtime.outcome-unknown,runtime.interrupted,runtime.cancelled,runtime.failed,runtime.wait-timeout,runtime.waiter-disconnected,runtime.parent-unresolved,runtime.target-removed,runtime.target-changed,runtime.target-unavailable,run.started,run.joined,run.fresh,run.busy,run.cancelling,activity.queued,activity.running,activity.success,activity.failed,activity.cancelled,activity.message,deploy.pull,job.queued,job.waiting,job.running,job.retrying,job.succeeded,job.partial,job.skipped,job.failed,job.needs_attention,job.canceled,container.die,container.oom,container.rename,container.unhealthy,image.untag,image.import,image.prune,volume.prune,network.prune,container.start,container.stop,container.restart,container.delete,container.create,container.scan,container.update,container.deploy,container.kill,container.pause,container.unpause,container.error,image.pull,image.load,image.tag,image.commit,image.delete,image.scan,image.error,image.vulnerability_scan,project.deploy,project.delete,project.start,project.stop,project.create,project.update,project.error,volume.create,volume.rename,volume.delete,volume.error,volume.file.create,volume.file.delete,volume.file.upload,volume.file.update,volume.workspace.update,volume.backup.create,volume.backup.delete,volume.backup.restore,volume.backup.restore_files,volume.backup.download,volume.backup.target.key_viewed,network.create,network.delete,network.connect,network.disconnect,network.error,system.prune,user.login,user.logout,system.auto_update,system.upgrade,environment.create,environment.connect,environment.disconnect,environment.update,environment.delete,environment.api_key.regenerated,environment.mtls.ca_generated,environment.mtls.cert_issued,environment.mtls.enroll,environment.mtls.download,webhook.create,webhook.update,webhook.delete,webhook.trigger,notification.send,autoheal.restarted,autoheal.restart-failed,autoheal.limit-reached,autoheal.interrupted,notification.delivery-failed,load.sustained,container.condition.down,container.condition.unhealthy,container.condition.restart-loop,container.condition.running-again"`
	ExecutorError       *string    `json:"executorError,omitempty"`
	ExecutorSettledAt   *time.Time `json:"executorSettledAt,omitempty"`
	Disposition         *string    `json:"disposition,omitempty"`
	DispositionBy       *string    `json:"dispositionBy,omitempty"`
	DispositionReason   *string    `json:"dispositionReason,omitempty"`
	DispositionAt       *time.Time `json:"dispositionAt,omitempty"`
	ReleaseAcknowledged bool       `json:"releaseAcknowledged"`
}

type RuntimeHubDetail struct {
	RuntimeExecutionDetail
	Recovery RuntimeRecoveryEvidence `json:"recovery"`
}
