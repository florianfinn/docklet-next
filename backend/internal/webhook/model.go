// Source: getarcaneapp/arcane/backend/internal/webhook/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories.

package webhook

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

const (
	WebhookTargetTypeContainer = "container"
	WebhookTargetTypeProject   = "project"
	WebhookTargetTypeUpdater   = "updater"

	WebhookActionTypeUpdate   = "update"
	WebhookActionTypeStart    = "start"
	WebhookActionTypeStop     = "stop"
	WebhookActionTypeRestart  = "restart"
	WebhookActionTypeRedeploy = "redeploy"
	WebhookActionTypeUp       = "up"
	WebhookActionTypeDown     = "down"
	WebhookActionTypeRun      = "run"
)

type Webhook struct {
	database.BaseModel

	Name            string     `json:"name" gorm:"column:name;not null"`
	TokenHash       string     `json:"-" gorm:"column:token_hash;not null"`
	TokenPrefix     string     `json:"tokenPrefix" gorm:"column:token_prefix;not null"`
	TargetType      string     `json:"targetType" gorm:"column:target_type;not null"`
	ActionType      string     `json:"actionType" gorm:"column:action_type;not null;default:''"`
	TargetID        string     `json:"targetId" gorm:"column:target_id;not null"`
	TargetRef       string     `json:"-" gorm:"column:target_ref;not null;default:''"`
	EnvironmentID   string     `json:"environmentId" gorm:"column:environment_id;not null;default:''"`
	Enabled         bool       `json:"enabled" gorm:"column:enabled;not null;default:true"`
	LastTriggeredAt *time.Time `json:"lastTriggeredAt,omitempty" gorm:"column:last_triggered_at"`
}

func (Webhook) TableName() string {
	return "webhooks"
}
