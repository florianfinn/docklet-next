// Source: getarcaneapp/arcane/types/event/event.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Stable event message code.

package event

import "time"

// Event represents an event in API responses.
type Event struct {
	// ID of the event.
	//
	// Required: true
	ID string `json:"id"`

	// Type of the event.
	//
	// Required: true
	Type        string `json:"type"`
	MessageCode string `json:"messageCode" enum:"run.started,run.joined,run.fresh,run.busy,activity.queued,activity.running,activity.success,activity.failed,activity.cancelled,activity.message,deploy.pull,job.queued,job.waiting,job.running,job.retrying,job.succeeded,job.partial,job.skipped,job.failed,job.needs_attention,job.canceled,container.die,container.oom,container.rename,container.unhealthy,image.untag,image.import,image.prune,volume.prune,network.prune,container.start,container.stop,container.restart,container.delete,container.create,container.scan,container.update,container.deploy,container.kill,container.pause,container.unpause,container.error,image.pull,image.load,image.tag,image.commit,image.delete,image.scan,image.error,image.vulnerability_scan,project.deploy,project.delete,project.start,project.stop,project.create,project.update,project.error,volume.create,volume.rename,volume.delete,volume.error,volume.file.create,volume.file.delete,volume.file.upload,volume.file.update,volume.workspace.update,volume.backup.create,volume.backup.delete,volume.backup.restore,volume.backup.restore_files,volume.backup.download,network.create,network.delete,network.connect,network.disconnect,network.error,system.prune,user.login,user.logout,system.auto_update,system.upgrade,environment.create,environment.connect,environment.disconnect,environment.update,environment.delete,environment.api_key.regenerated,environment.mtls.ca_generated,environment.mtls.cert_issued,environment.mtls.enroll,environment.mtls.download,webhook.create,webhook.update,webhook.delete,webhook.trigger,notification.send"`

	// Severity level of the event.
	//
	// Required: true
	Severity string `json:"severity"`

	// Title of the event.
	//
	// Required: true
	Title string `json:"title"`

	// Description of the event.
	//
	// Required: false
	Description string `json:"description,omitempty"`

	// ResourceType is the type of the resource associated with the event.
	//
	// Required: false
	ResourceType *string `json:"resourceType,omitempty"`

	// ResourceID is the ID of the resource associated with the event.
	//
	// Required: false
	ResourceID *string `json:"resourceId,omitempty"`

	// ResourceName is the name of the resource associated with the event.
	//
	// Required: false
	ResourceName *string `json:"resourceName,omitempty"`

	// UserID is the ID of the user who triggered the event.
	//
	// Required: false
	UserID *string `json:"userId,omitempty"`

	// Username is the username of the user who triggered the event.
	//
	// Required: false
	Username *string `json:"username,omitempty"`

	// EnvironmentID is the ID of the environment associated with the event.
	//
	// Required: false
	EnvironmentID *string `json:"environmentId,omitempty"`

	// Metadata contains additional key-value data associated with the event.
	//
	// Required: false
	Metadata map[string]any `json:"metadata,omitempty"`

	// Timestamp is when the event occurred.
	//
	// Required: true
	Timestamp time.Time `json:"timestamp"`

	// CreatedAt is the date and time at which the event was created.
	//
	// Required: true
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the date and time at which the event was last updated.
	//
	// Required: false
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

type ListReponse struct {
	// Events is a list of events.
	//
	// Required: true
	Events []Event `json:"events"`

	// Total number of events.
	//
	// Required: true
	Total int `json:"total"`

	// Page number of the current result set.
	//
	// Required: true
	Page int `json:"page"`

	// PageSize is the number of events per page.
	//
	// Required: true
	PageSize int `json:"pageSize"`

	// TotalPages is the total number of pages.
	//
	// Required: true
	TotalPages int `json:"totalPages"`

	// HasNext indicates if there is a next page.
	//
	// Required: true
	HasNext bool `json:"hasNext"`

	// HasPrevious indicates if there is a previous page.
	//
	// Required: true
	HasPrevious bool `json:"hasPrevious"`
}
