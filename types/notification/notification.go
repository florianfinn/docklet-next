// Source: getarcaneapp/arcane/types/notification/notification.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package notification

import "github.com/florianfinn/docklet-next/types/vulnerability"

import (
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/florianfinn/docklet-next/types/system"
)

// Provider is the type for notification provider identifiers.
type Provider string

const (
	// NotificationProviderDiscord is the builtin Discord notification provider.
	NotificationProviderDiscord Provider = "discord"

	// NotificationProviderEmail is the builtin Email notification provider.
	NotificationProviderEmail Provider = "email"

	// NotificationProviderTelegram is the builtin Telegram notification provider.
	NotificationProviderTelegram Provider = "telegram"

	// NotificationProviderSignal is the builtin Signal notification provider.
	NotificationProviderSignal Provider = "signal"

	// NotificationProviderSlack is the builtin Slack notification provider.
	NotificationProviderSlack Provider = "slack"

	// NotificationProviderNtfy is the builtin Ntfy notification provider.
	NotificationProviderNtfy Provider = "ntfy"

	// NotificationProviderPushover is the builtin Pushover notification provider.
	NotificationProviderPushover Provider = "pushover"

	// NotificationProviderGotify is the builtin Gotify notification provider.
	NotificationProviderGotify Provider = "gotify"

	// NotificationProviderMatrix is the builtin Matrix webhook notification provider.
	NotificationProviderMatrix Provider = "matrix"

	// NotificationProviderGoogleChat is the builtin Google Chat notification provider.
	NotificationProviderGoogleChat Provider = "googlechat"

	// NotificationProviderGeneric is the builtin Generic webhook notification provider.
	NotificationProviderGeneric Provider = "generic"

	DispatchKindImageUpdate          DispatchKind = "image_update"
	DispatchKindBatchImageUpdate     DispatchKind = "batch_image_update"
	DispatchKindContainerUpdate      DispatchKind = "container_update"
	DispatchKindBatchContainerUpdate DispatchKind = "batch_container_update"
	DispatchKindVulnerabilityFound   DispatchKind = "vulnerability_found"
	DispatchKindVulnerabilityScan    DispatchKind = "vulnerability_scan"
	DispatchKindPruneReport          DispatchKind = "prune_report"
	DispatchKindAutoHeal             DispatchKind = "auto_heal"
	DispatchKindBackupFailed         DispatchKind = "backup_failed"
)

type Update struct {
	// Provider is the notification provider type.
	//
	// Required: true
	Provider Provider `json:"provider" binding:"required"`

	// Enabled indicates if the notification provider is enabled.
	//
	// Required: true
	Enabled bool `json:"enabled"`

	// Config contains the provider-specific configuration.
	//
	// Required: true
	Config base.JsonObject `json:"config" binding:"required"`
}

type Response struct {
	// ID is the unique identifier of the notification settings.
	//
	// Required: true
	ID uint `json:"id"`

	// Provider is the notification provider type.
	//
	// Required: true
	Provider Provider `json:"provider"`

	// Enabled indicates if the notification provider is enabled.
	//
	// Required: true
	Enabled bool `json:"enabled"`

	// Config contains the provider-specific configuration.
	//
	// Required: true
	Config         base.JsonObject `json:"config"`
	CredentialsSet []string        `json:"credentialsSet"`
}

// TestResponse is the result of a test notification send.
type TestResponse struct {
	// Message describes the test outcome.
	Code   string `json:"code" enum:"notification.test-sent,notification.test-failed"`
	Reason string `json:"reason,omitempty" enum:"delivery-unreachable,delivery-rejected,delivery-auth,delivery-invalid"`

	// Warning explains why a real notification of the tested kind would not
	// send (provider disabled or event unsubscribed). Empty when none applies.
	Warning string `json:"warning,omitempty" enum:"notification.target-disabled"`
}

type DispatchKind string

type DispatchImageUpdate struct {
	ImageRef   string               `json:"imageRef"`
	UpdateInfo imageupdate.Response `json:"updateInfo"`
}

type DispatchBatchImageUpdate struct {
	Updates map[string]*imageupdate.Response `json:"updates"`
}

type DispatchContainerUpdate struct {
	ContainerName string `json:"containerName"`
	ImageRef      string `json:"imageRef"`
	OldDigest     string `json:"oldDigest,omitempty"`
	NewDigest     string `json:"newDigest,omitempty"`
}

type DispatchBatchContainerUpdate struct {
	Updates []DispatchContainerUpdate `json:"updates"`
}

type DispatchVulnerabilityFound struct {
	CVEID            string `json:"cveId"`
	CVELink          string `json:"cveLink"`
	Severity         string `json:"severity"`
	ImageName        string `json:"imageName"`
	FixedVersion     string `json:"fixedVersion,omitempty"`
	PkgName          string `json:"pkgName,omitempty"`
	InstalledVersion string `json:"installedVersion,omitempty"`
}

type DispatchPruneReport struct {
	Result system.PruneAllResult `json:"result"`
}

type DispatchAutoHeal struct {
	EventID       string `json:"eventId" maxLength:"64"`
	Outcome       string `json:"outcome" enum:"restarted,restart-failed,limit-reached"`
	Container     string `json:"container" maxLength:"128"`
	Stack         string `json:"stack,omitempty" maxLength:"128"`
	Restarts      int    `json:"restarts" minimum:"0" maximum:"100"`
	WindowMinutes int    `json:"windowMinutes" minimum:"1" maximum:"1440"`
	ContainerName string `json:"containerName"`
	ContainerID   string `json:"containerId"`
}

type DispatchVulnerabilityScan struct {
	HostName    string                     `json:"hostName" maxLength:"256"`
	Outcome     string                     `json:"outcome" enum:"completed,partial,cancelled,failed,interrupted"`
	NewCritical int                        `json:"newCritical" minimum:"0"`
	NewHigh     int                        `json:"newHigh" minimum:"0"`
	NewFindings []vulnerability.NewFinding `json:"newFindings" maxItems:"50"`
}

type DispatchBackupFailed struct {
	EventID    string `json:"eventId" maxLength:"64"`
	Operation  string `json:"operation" enum:"backup,restore"`
	Volume     string `json:"volume" maxLength:"128"`
	Reason     string `json:"reason"`
	ActivityID string `json:"activityId,omitempty" format:"uuid"`
}

type DispatchRequest struct {
	BackupFailed      *DispatchBackupFailed      `json:"backupFailed,omitempty"`
	VulnerabilityScan *DispatchVulnerabilityScan `json:"vulnerabilityScan,omitempty"`

	Kind                 DispatchKind                  `json:"kind" enum:"image_update,batch_image_update,container_update,batch_container_update,prune_report,auto_heal,backup_failed"`
	ImageUpdate          *DispatchImageUpdate          `json:"imageUpdate,omitempty"`
	BatchImageUpdate     *DispatchBatchImageUpdate     `json:"batchImageUpdate,omitempty"`
	ContainerUpdate      *DispatchContainerUpdate      `json:"containerUpdate,omitempty"`
	BatchContainerUpdate *DispatchBatchContainerUpdate `json:"batchContainerUpdate,omitempty"`
	VulnerabilityFound   *DispatchVulnerabilityFound   `json:"vulnerabilityFound,omitempty"`
	PruneReport          *DispatchPruneReport          `json:"pruneReport,omitempty"`
	AutoHeal             *DispatchAutoHeal             `json:"autoHeal,omitempty"`
}

type DispatchResponse struct {
	Message   string `json:"message"`
	Delivered int    `json:"delivered"`
}
