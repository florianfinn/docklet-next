// Source: getarcaneapp/arcane/types/webhook/webhook.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed GitOps and saved Git repositories.

package webhook

import "time"

// CreateInput is the request body for creating a webhook.
type CreateInput struct {
	Name       string `json:"name" minLength:"1" maxLength:"255" doc:"Human-readable name for this webhook" unorm:"nfc" trim:"true"`
	TargetType string `json:"targetType" doc:"Resource type this webhook targets: 'container', 'project', or 'updater'" enum:"container,project,updater"`
	ActionType string `json:"actionType" doc:"Action to run for the selected target type. Supported values depend on targetType." enum:"update,start,stop,restart,redeploy,up,down,run,sync"`
	TargetID   string `json:"targetId" doc:"Container ID, project ID,  to target. Leave empty for 'updater' webhooks."`
}

// Summary is returned in list responses — the token is masked.
type Summary struct {
	ID              string     `json:"id" doc:"Webhook ID"`
	Name            string     `json:"name" doc:"Webhook name"`
	TokenPrefix     string     `json:"tokenPrefix" doc:"Masked token prefix for identification"`
	TargetType      string     `json:"targetType" doc:"Target type: 'container', 'project', or 'updater'"`
	ActionType      string     `json:"actionType" doc:"Action type resolved for this webhook"`
	TargetID        string     `json:"targetId" doc:"Target resource ID"`
	TargetName      string     `json:"targetName,omitempty" doc:"Resolved target resource name when available"`
	EnvironmentID   string     `json:"environmentId" doc:"Environment ID"`
	Enabled         bool       `json:"enabled" doc:"Whether the webhook is active"`
	LastTriggeredAt *time.Time `json:"lastTriggeredAt,omitempty" doc:"Timestamp of last successful trigger"`
	CreatedAt       time.Time  `json:"createdAt" doc:"Creation timestamp"`
}

// UpdateInput is the request body for updating a webhook.
type UpdateInput struct {
	Enabled bool `json:"enabled" doc:"Whether the webhook is active"`
}

// Created is returned once when a webhook is first created, including the raw token.
type Created struct {
	ID         string    `json:"id" doc:"Webhook ID"`
	Name       string    `json:"name" doc:"Webhook name"`
	Token      string    `json:"token" doc:"Full webhook token — store this securely, it will not be shown again"`
	TargetType string    `json:"targetType" doc:"Target type"`
	ActionType string    `json:"actionType" doc:"Action type"`
	TargetID   string    `json:"targetId" doc:"Target resource ID"`
	CreatedAt  time.Time `json:"createdAt" doc:"Creation timestamp"`
}
