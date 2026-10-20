// Source: florianfinn/docklet-next/types/activity
// Revision: 65c9e0dceb6f994ac884904e2a6b7348f75580fa
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: Stable wire-code catalogue for runs and events.

package activity

// Code identifies a stable run or event message independently of display text.
type Code string

const (
	CodeRunStarted                        Code = "run.started"
	CodeRunJoined                         Code = "run.joined"
	CodeRunFresh                          Code = "run.fresh"
	CodeRunBusy                           Code = "run.busy"
	CodeRunCancelling                     Code = "run.cancelling"
	CodeActivityQueued                    Code = "activity.queued"
	CodeActivityRunning                   Code = "activity.running"
	CodeActivitySuccess                   Code = "activity.success"
	CodeActivityFailed                    Code = "activity.failed"
	CodeActivityCancelled                 Code = "activity.cancelled"
	CodeActivityMessage                   Code = "activity.message"
	CodeDeployPull                        Code = "deploy.pull"
	CodeJobQueued                         Code = "job.queued"
	CodeJobWaiting                        Code = "job.waiting"
	CodeJobRunning                        Code = "job.running"
	CodeJobRetrying                       Code = "job.retrying"
	CodeJobSucceeded                      Code = "job.succeeded"
	CodeJobPartial                        Code = "job.partial"
	CodeJobSkipped                        Code = "job.skipped"
	CodeJobFailed                         Code = "job.failed"
	CodeJobNeedsAttention                 Code = "job.needs_attention"
	CodeJobCanceled                       Code = "job.canceled"
	CodeEventContainerDie                 Code = "container.die"
	CodeEventContainerOOM                 Code = "container.oom"
	CodeEventContainerRename              Code = "container.rename"
	CodeEventContainerUnhealthy           Code = "container.unhealthy"
	CodeEventImageUntag                   Code = "image.untag"
	CodeEventImageImport                  Code = "image.import"
	CodeEventImagePrune                   Code = "image.prune"
	CodeEventVolumePrune                  Code = "volume.prune"
	CodeEventNetworkPrune                 Code = "network.prune"
	CodeEventContainerStart               Code = "container.start"
	CodeEventContainerStop                Code = "container.stop"
	CodeEventContainerRestart             Code = "container.restart"
	CodeEventContainerDelete              Code = "container.delete"
	CodeEventContainerCreate              Code = "container.create"
	CodeEventContainerScan                Code = "container.scan"
	CodeEventContainerUpdate              Code = "container.update"
	CodeEventContainerDeploy              Code = "container.deploy"
	CodeEventContainerKill                Code = "container.kill"
	CodeEventContainerPause               Code = "container.pause"
	CodeEventContainerUnpause             Code = "container.unpause"
	CodeEventContainerError               Code = "container.error"
	CodeEventImagePull                    Code = "image.pull"
	CodeEventImageLoad                    Code = "image.load"
	CodeEventImageTag                     Code = "image.tag"
	CodeEventImageCommit                  Code = "image.commit"
	CodeEventImageDelete                  Code = "image.delete"
	CodeEventImageScan                    Code = "image.scan"
	CodeEventImageError                   Code = "image.error"
	CodeEventImageVulnerabilityScan       Code = "image.vulnerability_scan"
	CodeEventProjectDeploy                Code = "project.deploy"
	CodeEventProjectDelete                Code = "project.delete"
	CodeEventProjectStart                 Code = "project.start"
	CodeEventProjectStop                  Code = "project.stop"
	CodeEventProjectCreate                Code = "project.create"
	CodeEventProjectUpdate                Code = "project.update"
	CodeEventProjectError                 Code = "project.error"
	CodeEventVolumeCreate                 Code = "volume.create"
	CodeEventVolumeRename                 Code = "volume.rename"
	CodeEventVolumeDelete                 Code = "volume.delete"
	CodeEventVolumeError                  Code = "volume.error"
	CodeEventVolumeFileCreate             Code = "volume.file.create"
	CodeEventVolumeFileDelete             Code = "volume.file.delete"
	CodeEventVolumeFileUpload             Code = "volume.file.upload"
	CodeEventVolumeFileUpdate             Code = "volume.file.update"
	CodeEventVolumeWorkspaceUpdate        Code = "volume.workspace.update"
	CodeEventVolumeBackupCreate           Code = "volume.backup.create"
	CodeEventVolumeBackupDelete           Code = "volume.backup.delete"
	CodeEventVolumeBackupRestore          Code = "volume.backup.restore"
	CodeEventVolumeBackupRestoreFiles     Code = "volume.backup.restore_files"
	CodeEventVolumeBackupDownload         Code = "volume.backup.download"
	CodeEventNetworkCreate                Code = "network.create"
	CodeEventNetworkDelete                Code = "network.delete"
	CodeEventNetworkConnect               Code = "network.connect"
	CodeEventNetworkDisconnect            Code = "network.disconnect"
	CodeEventNetworkError                 Code = "network.error"
	CodeEventSystemPrune                  Code = "system.prune"
	CodeEventUserLogin                    Code = "user.login"
	CodeEventUserLogout                   Code = "user.logout"
	CodeEventSystemAutoUpdate             Code = "system.auto_update"
	CodeEventSystemUpgrade                Code = "system.upgrade"
	CodeEventEnvironmentCreate            Code = "environment.create"
	CodeEventEnvironmentConnect           Code = "environment.connect"
	CodeEventEnvironmentDisconnect        Code = "environment.disconnect"
	CodeEventEnvironmentUpdate            Code = "environment.update"
	CodeEventEnvironmentDelete            Code = "environment.delete"
	CodeEventEnvironmentAccessRegenerated Code = "environment.api_key.regenerated"
	CodeEventEnvironmentMTLSCAGenerated   Code = "environment.mtls.ca_generated"
	CodeEventEnvironmentMTLSCertIssued    Code = "environment.mtls.cert_issued"
	CodeEventEnvironmentMTLSEnroll        Code = "environment.mtls.enroll"
	CodeEventEnvironmentMTLSDownload      Code = "environment.mtls.download"
	CodeEventWebhookCreate                Code = "webhook.create"
	CodeEventWebhookUpdate                Code = "webhook.update"
	CodeEventWebhookDelete                Code = "webhook.delete"
	CodeEventWebhookTrigger               Code = "webhook.trigger"
	CodeEventNotificationSend             Code = "notification.send"
)

// Values is the complete wire catalogue used by the API contract.
func Values() []Code {
	return []Code{
		CodeRunRequestMismatch,
		CodeRuntimeOutcomeUnknown,
		CodeRuntimeInterrupted,
		CodeRuntimeCancelled,
		CodeRuntimeFailed,
		CodeRuntimeWaitTimeout,
		CodeRuntimeWaiterDisconnected,
		CodeRuntimeParentUnresolved,
		CodeRuntimeTargetRemoved,
		CodeRuntimeTargetChanged,
		CodeRuntimeTargetUnavailable,

		CodeRunStarted,
		CodeRunJoined,
		CodeRunFresh,
		CodeRunBusy,
		CodeRunCancelling,
		CodeActivityQueued,
		CodeActivityRunning,
		CodeActivitySuccess,
		CodeActivityFailed,
		CodeActivityCancelled,
		CodeActivityMessage,
		CodeDeployPull,
		CodeJobQueued,
		CodeJobWaiting,
		CodeJobRunning,
		CodeJobRetrying,
		CodeJobSucceeded,
		CodeJobPartial,
		CodeJobSkipped,
		CodeJobFailed,
		CodeJobNeedsAttention,
		CodeJobCanceled,
		CodeEventContainerDie,
		CodeEventContainerOOM,
		CodeEventContainerRename,
		CodeEventContainerUnhealthy,
		CodeEventImageUntag,
		CodeEventImageImport,
		CodeEventImagePrune,
		CodeEventVolumePrune,
		CodeEventNetworkPrune,
		CodeEventContainerStart,
		CodeEventContainerStop,
		CodeEventContainerRestart,
		CodeEventContainerDelete,
		CodeEventContainerCreate,
		CodeEventContainerScan,
		CodeEventContainerUpdate,
		CodeEventContainerDeploy,
		CodeEventContainerKill,
		CodeEventContainerPause,
		CodeEventContainerUnpause,
		CodeEventContainerError,
		CodeEventImagePull,
		CodeEventImageLoad,
		CodeEventImageTag,
		CodeEventImageCommit,
		CodeEventImageDelete,
		CodeEventImageScan,
		CodeEventImageError,
		CodeEventImageVulnerabilityScan,
		CodeEventProjectDeploy,
		CodeEventProjectDelete,
		CodeEventProjectStart,
		CodeEventProjectStop,
		CodeEventProjectCreate,
		CodeEventProjectUpdate,
		CodeEventProjectError,
		CodeEventVolumeCreate,
		CodeEventVolumeRename,
		CodeEventVolumeDelete,
		CodeEventVolumeError,
		CodeEventVolumeFileCreate,
		CodeEventVolumeFileDelete,
		CodeEventVolumeFileUpload,
		CodeEventVolumeFileUpdate,
		CodeEventVolumeWorkspaceUpdate,
		CodeEventVolumeBackupCreate,
		CodeEventVolumeBackupDelete,
		CodeEventVolumeBackupRestore,
		CodeEventVolumeBackupRestoreFiles,
		CodeEventVolumeBackupDownload,
		CodeEventNetworkCreate,
		CodeEventNetworkDelete,
		CodeEventNetworkConnect,
		CodeEventNetworkDisconnect,
		CodeEventNetworkError,
		CodeEventSystemPrune,
		CodeEventUserLogin,
		CodeEventUserLogout,
		CodeEventSystemAutoUpdate,
		CodeEventSystemUpgrade,
		CodeEventEnvironmentCreate,
		CodeEventEnvironmentConnect,
		CodeEventEnvironmentDisconnect,
		CodeEventEnvironmentUpdate,
		CodeEventEnvironmentDelete,
		CodeEventEnvironmentAccessRegenerated,
		CodeEventEnvironmentMTLSCAGenerated,
		CodeEventEnvironmentMTLSCertIssued,
		CodeEventEnvironmentMTLSEnroll,
		CodeEventEnvironmentMTLSDownload,
		CodeEventWebhookCreate,
		CodeEventWebhookUpdate,
		CodeEventWebhookDelete,
		CodeEventWebhookTrigger,
		CodeEventNotificationSend,
	}
}

func Valid(value string) bool {
	for _, code := range Values() {
		if value == string(code) {
			return true
		}
	}
	return false
}
