// Source: getarcaneapp/arcane/backend/internal/activity/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; run gate, service effects, coded steps, output columns and compact runtime model registration.

package activity

import (
	"database/sql/driver"
	"time"

	"github.com/florianfinn/docklet-next/types/activity"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

type Activity struct {
	database.BaseModel

	EnvironmentID        string             `json:"environmentId" gorm:"column:environment_id;not null;index" sortable:"true"`
	BatchID              *string            `json:"batchId,omitempty" gorm:"column:batch_id;index"`
	Type                 activity.Type      `json:"type" gorm:"column:type;not null;index" sortable:"true"`
	Status               activity.Status    `json:"status" gorm:"column:status;not null;index" sortable:"true"`
	ResourceType         *string            `json:"resourceType,omitempty" gorm:"column:resource_type;index" sortable:"true"`
	ResourceID           *string            `json:"resourceId,omitempty" gorm:"column:resource_id;index"`
	ResourceName         *string            `json:"resourceName,omitempty" gorm:"column:resource_name" sortable:"true"`
	Progress             *int               `json:"progress,omitempty" gorm:"column:progress"`
	Step                 string             `json:"step,omitempty" gorm:"column:step"`
	LatestMessage        string             `json:"latestMessage,omitempty" gorm:"column:latest_message"`
	StartedByUserID      *string            `json:"startedByUserId,omitempty" gorm:"column:started_by_user_id;index"`
	StartedByUsername    *string            `json:"startedByUsername,omitempty" gorm:"column:started_by_username"`
	StartedByDisplayName *string            `json:"startedByDisplayName,omitempty" gorm:"column:started_by_display_name"`
	StartedAt            time.Time          `json:"startedAt" gorm:"column:started_at;not null" sortable:"true"`
	EndedAt              *time.Time         `json:"endedAt,omitempty" gorm:"column:ended_at" sortable:"true"`
	DurationMs           *int64             `json:"durationMs,omitempty" gorm:"column:duration_ms" sortable:"true"`
	Error                *string            `json:"error,omitempty" gorm:"column:error"`
	Metadata             database.JSON      `json:"metadata,omitempty" gorm:"type:text"`
	MessageCode          string             `json:"messageCode,omitempty" gorm:"column:message_code"`
	AffectedServices     database.JSON      `json:"affectedServices,omitempty" gorm:"column:affected_services;type:text"`
	GateKey              string             `json:"-" gorm:"column:gate_key;index"`
	GateScope            string             `json:"-" gorm:"column:gate_scope;index"`
	Output               OutputLines        `json:"-" gorm:"column:output;type:text;not null;default:'[]'"`
	OutputTruncated      bool               `json:"-" gorm:"column:output_truncated;not null;default:false"`
	RuntimeOutput        RuntimeOutputLines `json:"-" gorm:"column:runtime_output;type:text;not null;default:'[]'"`
	RuntimeStepTrace     RuntimeStepTrace   `json:"-" gorm:"column:runtime_step_trace;type:text;not null;default:'[]'"`
}

type OutputLines []activity.OutputLine

func (lines OutputLines) Value() (driver.Value, error) {
	if lines == nil {
		lines = OutputLines{}
	}
	return database.JSONValue(false, []activity.OutputLine(lines))
}

func (lines *OutputLines) Scan(value any) error {
	return database.JSONScan(lines, value)
}

func (Activity) TableName() string {
	return "activities"
}

type ActivityMessage struct {
	database.BaseModel

	ActivityID    string                `json:"activityId" gorm:"column:activity_id;not null;index"`
	Level         activity.MessageLevel `json:"level" gorm:"column:level;not null"`
	Message       string                `json:"message" gorm:"column:message;not null"`
	Payload       database.JSON         `json:"payload,omitempty" gorm:"type:text"`
	Code          string                `json:"code,omitempty" gorm:"column:code"`
	Step          string                `json:"step,omitempty" gorm:"column:step"`
	Activity      *Activity             `json:"-" gorm:"foreignKey:ActivityID;constraint:OnDelete:CASCADE"`
	RuntimeRemote bool                  `json:"-" gorm:"column:runtime_remote;not null;default:false"`
}

func (ActivityMessage) TableName() string {
	return "activity_messages"
}

// ActivitySyncReceipt persists applied run versions independently of history pruning.
type ActivitySyncReceipt struct {
	RunID     string     `gorm:"column:run_id;primaryKey;not null"`
	UpdatedAt time.Time  `gorm:"column:updated_at;not null"`
	SyncedAt  *time.Time `gorm:"column:synced_at"`
}

func (ActivitySyncReceipt) TableName() string { return "activity_sync_receipts" }

// Models returns the persistent activity schema for integration fixtures and the core registry.
func Models() []any {
	return []any{&Activity{}, &ActivityMessage{}, &ActivitySyncReceipt{},
		&RuntimeReceipt{}, &RuntimeKeyBinding{}, &RuntimeStoreGeneration{}, &RuntimeRemovedTarget{}, &RuntimeExecutorBinding{}}
}
