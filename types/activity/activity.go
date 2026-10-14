// Source: getarcaneapp/arcane/types/activity/activity.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Functional cut: removed standalone build workshop; coded run trace and service effects.

package activity

import "time"

type Status string

const (
	TypeJobRun      Type   = "job_run"
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSuccess   Status = "success"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"

	TypeImagePull         Type = "image_pull"
	TypeImageUpdateCheck  Type = "image_update_check"
	TypeProjectPull       Type = "project_pull"
	TypeProjectBuild      Type = "project_build"
	TypeProjectDeploy     Type = "project_deploy"
	TypeProjectRedeploy   Type = "project_redeploy"
	TypeProjectDown       Type = "project_down"
	TypeProjectRestart    Type = "project_restart"
	TypeProjectDestroy    Type = "project_destroy"
	TypeContainerStart    Type = "container_start"
	TypeContainerStop     Type = "container_stop"
	TypeContainerRestart  Type = "container_restart"
	TypeContainerRedeploy Type = "container_redeploy"
	TypeContainerEdit     Type = "container_edit"
	TypeContainerDelete   Type = "container_delete"
	TypeContainerKill     Type = "container_kill"
	TypeContainerPause    Type = "container_pause"
	TypeContainerUnpause  Type = "container_unpause"
	TypeVulnerabilityScan Type = "vulnerability_scan"
	TypeImagePatch        Type = "image_patch"
	TypeAutoUpdate        Type = "auto_update"
	TypeSystemPrune       Type = "system_prune"
	TypeResourceAction    Type = "resource_action"

	MessageLevelInfo    MessageLevel = "info"
	MessageLevelWarning MessageLevel = "warning"
	MessageLevelError   MessageLevel = "error"
	MessageLevelSuccess MessageLevel = "success"
)

type Type string

type MessageLevel string

type Activity struct {
	ID                    string         `json:"id"`
	EnvironmentID         string         `json:"environmentId"`
	BatchID               *string        `json:"batchId,omitempty"`
	SourceEnvironmentID   string         `json:"sourceEnvironmentId,omitempty"`
	SourceEnvironmentName string         `json:"sourceEnvironmentName,omitempty"`
	Type                  Type           `json:"type"`
	Status                Status         `json:"status"`
	ResourceType          *string        `json:"resourceType,omitempty"`
	ResourceID            *string        `json:"resourceId,omitempty"`
	ResourceName          *string        `json:"resourceName,omitempty"`
	Progress              *int           `json:"progress,omitempty"`
	Step                  string         `json:"step,omitempty"`
	LatestMessage         string         `json:"latestMessage,omitempty"`
	MessageCode           string         `json:"messageCode,omitempty" enum:"run.started,run.joined,run.fresh,run.busy,run.cancelling,activity.queued,activity.running,activity.success,activity.failed,activity.cancelled,activity.cancel_requested,activity.message,deploy.pull,job.queued,job.waiting,job.running,job.retrying,job.succeeded,job.partial,job.skipped,job.failed,job.needs_attention,job.canceled,container.die,container.oom,container.rename,container.unhealthy,image.untag,image.import,image.prune,volume.prune,network.prune,container.start,container.stop,container.restart,container.delete,container.create,container.scan,container.update,container.deploy,container.kill,container.pause,container.unpause,container.error,image.pull,image.load,image.tag,image.commit,image.delete,image.scan,image.error,image.vulnerability_scan,project.deploy,project.delete,project.start,project.stop,project.create,project.update,project.error,volume.create,volume.rename,volume.delete,volume.error,volume.file.create,volume.file.delete,volume.file.upload,volume.file.update,volume.workspace.update,volume.backup.create,volume.backup.delete,volume.backup.restore,volume.backup.restore_files,volume.backup.download,network.create,network.delete,network.connect,network.disconnect,network.error,system.prune,user.login,user.logout,system.auto_update,system.upgrade,environment.create,environment.connect,environment.disconnect,environment.update,environment.delete,environment.api_key.regenerated,environment.mtls.ca_generated,environment.mtls.cert_issued,environment.mtls.enroll,environment.mtls.download,webhook.create,webhook.update,webhook.delete,webhook.trigger,notification.send"`
	CancelRequestedAt     *time.Time     `json:"cancelRequestedAt,omitempty"`
	AffectedServices      []string       `json:"affectedServices,omitempty"`
	StartedBy             *StartedBy     `json:"startedBy,omitempty"`
	StartedAt             time.Time      `json:"startedAt"`
	EndedAt               *time.Time     `json:"endedAt,omitempty"`
	DurationMs            *int64         `json:"durationMs,omitempty"`
	Error                 *string        `json:"error,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             *time.Time     `json:"updatedAt,omitempty"`
}

type StartedBy struct {
	UserID      string `json:"userId,omitempty"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName,omitempty"`
}

type Message struct {
	ID         string         `json:"id"`
	ActivityID string         `json:"activityId"`
	Level      MessageLevel   `json:"level"`
	Message    string         `json:"message"`
	Code       string         `json:"code,omitempty" enum:"run.started,run.joined,run.fresh,run.busy,run.cancelling,activity.queued,activity.running,activity.success,activity.failed,activity.cancelled,activity.cancel_requested,activity.message,deploy.pull,job.queued,job.waiting,job.running,job.retrying,job.succeeded,job.partial,job.skipped,job.failed,job.needs_attention,job.canceled,container.die,container.oom,container.rename,container.unhealthy,image.untag,image.import,image.prune,volume.prune,network.prune,container.start,container.stop,container.restart,container.delete,container.create,container.scan,container.update,container.deploy,container.kill,container.pause,container.unpause,container.error,image.pull,image.load,image.tag,image.commit,image.delete,image.scan,image.error,image.vulnerability_scan,project.deploy,project.delete,project.start,project.stop,project.create,project.update,project.error,volume.create,volume.rename,volume.delete,volume.error,volume.file.create,volume.file.delete,volume.file.upload,volume.file.update,volume.workspace.update,volume.backup.create,volume.backup.delete,volume.backup.restore,volume.backup.restore_files,volume.backup.download,network.create,network.delete,network.connect,network.disconnect,network.error,system.prune,user.login,user.logout,system.auto_update,system.upgrade,environment.create,environment.connect,environment.disconnect,environment.update,environment.delete,environment.api_key.regenerated,environment.mtls.ca_generated,environment.mtls.cert_issued,environment.mtls.enroll,environment.mtls.download,webhook.create,webhook.update,webhook.delete,webhook.trigger,notification.send"`
	Step       string         `json:"step,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
}

type Detail struct {
	Activity  Activity         `json:"activity"`
	Messages  []Message        `json:"messages"`
	StepTrace []StepTraceEntry `json:"stepTrace"`
}

type StepTraceEntry struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Status    Status     `json:"status" enum:"queued,running,success,failed,cancelled"`
	Code      string     `json:"code,omitempty" enum:"run.started,run.joined,run.fresh,run.busy,run.cancelling,activity.queued,activity.running,activity.success,activity.failed,activity.cancelled,activity.cancel_requested,activity.message,deploy.pull,job.queued,job.waiting,job.running,job.retrying,job.succeeded,job.partial,job.skipped,job.failed,job.needs_attention,job.canceled,container.die,container.oom,container.rename,container.unhealthy,image.untag,image.import,image.prune,volume.prune,network.prune,container.start,container.stop,container.restart,container.delete,container.create,container.scan,container.update,container.deploy,container.kill,container.pause,container.unpause,container.error,image.pull,image.load,image.tag,image.commit,image.delete,image.scan,image.error,image.vulnerability_scan,project.deploy,project.delete,project.start,project.stop,project.create,project.update,project.error,volume.create,volume.rename,volume.delete,volume.error,volume.file.create,volume.file.delete,volume.file.upload,volume.file.update,volume.workspace.update,volume.backup.create,volume.backup.delete,volume.backup.restore,volume.backup.restore_files,volume.backup.download,network.create,network.delete,network.connect,network.disconnect,network.error,system.prune,user.login,user.logout,system.auto_update,system.upgrade,environment.create,environment.connect,environment.disconnect,environment.update,environment.delete,environment.api_key.regenerated,environment.mtls.ca_generated,environment.mtls.cert_issued,environment.mtls.enroll,environment.mtls.download,webhook.create,webhook.update,webhook.delete,webhook.trigger,notification.send"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

type StreamEvent struct {
	Type          string     `json:"type"`
	EnvironmentID string     `json:"environmentId,omitempty"`
	ActivityID    string     `json:"activityId,omitempty"`
	Activity      *Activity  `json:"activity,omitempty"`
	Activities    []Activity `json:"activities,omitempty"`
	Message       *Message   `json:"message,omitempty"`
	Error         string     `json:"error,omitempty"`
	Timestamp     time.Time  `json:"timestamp"`
	// ActivityType is the type of the owning activity for events that carry no
	// activity payload. It is an internal hint used to skip visibility rereads
	// for ordinary activities and is never serialized.
	ActivityType Type `json:"-"`
}

type ClearHistoryResult struct {
	Deleted int64 `json:"deleted"`
}
