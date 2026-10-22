// Source: docklet-next/docs/design/runtime-control.md
// Revision: c641dc24b3f364ac5e4ec03bc9c5dfe2a10d5f61a6bcba825785f9dc351a9e50
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Compact receipt, key-binding and local-generation models.

package activity

import "time"

// RuntimeReceipt keeps compact evidence independently of activity/output history.
// A not_received tombstone has no invented caller, target or admission timestamps.
// StoreGeneration identifies this database; ExecutorGeneration binds the executor.
type RuntimeReceipt struct {
	RunID                string     `gorm:"column:run_id;primaryKey;not null"`
	Kind                 string     `gorm:"column:kind;not null"`
	StoreGeneration      string     `gorm:"column:store_generation;not null"`
	ManagerID            string     `gorm:"column:manager_id;not null"`
	RegistrationID       string     `gorm:"column:registration_id;not null"`
	EnvironmentID        string     `gorm:"column:environment_id;not null"`
	ExecutorGeneration   string     `gorm:"column:executor_generation;not null"`
	RequestDigest        string     `gorm:"column:request_digest;not null"`
	CallerID             *string    `gorm:"column:caller_id"`
	CallerName           *string    `gorm:"column:caller_name"`
	OperationID          *string    `gorm:"column:operation_id"`
	TargetKind           *string    `gorm:"column:target_kind"`
	TargetID             *string    `gorm:"column:target_id"`
	TargetName           *string    `gorm:"column:target_name"`
	ParentProjectID      *string    `gorm:"column:parent_project_id"`
	GateScope            *string    `gorm:"column:gate_scope"`
	GateKey              *string    `gorm:"column:gate_key"`
	AdmittedAt           *time.Time `gorm:"column:admitted_at"`
	DispatchState        *string    `gorm:"column:dispatch_state"`
	DispatchedAt         *time.Time `gorm:"column:dispatched_at"`
	ExecutorState        *string    `gorm:"column:executor_state"`
	UnknownReason        *string    `gorm:"column:unknown_reason"`
	ExecutorResultCode   *string    `gorm:"column:executor_result_code"`
	ExecutorError        *string    `gorm:"column:executor_error"`
	ExecutorStartedAt    *time.Time `gorm:"column:executor_started_at"`
	ExecutorSettledAt    *time.Time `gorm:"column:executor_settled_at"`
	CancelRequested      bool       `gorm:"column:cancel_requested;not null"`
	CancelRequestedAt    *time.Time `gorm:"column:cancel_requested_at"`
	LastReconciledAt     *time.Time `gorm:"column:last_reconciled_at"`
	HubDisposition       *string    `gorm:"column:hub_disposition"`
	HubDispositionBy     *string    `gorm:"column:hub_disposition_by"`
	HubDispositionReason *string    `gorm:"column:hub_disposition_reason"`
	ReleaseAcknowledged  bool       `gorm:"column:release_acknowledged;not null"`
	HubDispositionAt     *time.Time `gorm:"column:hub_disposition_at"`
	SettledAt            *time.Time `gorm:"column:settled_at"`
	FinalWriteAt         time.Time  `gorm:"column:final_write_at;not null"`
	OutputNextSeq        uint64     `gorm:"column:output_next_seq;not null;default:1"`
}

func (RuntimeReceipt) TableName() string { return "runtime_receipts" }

// RuntimeKeyBinding attributes each original or joining key to its own caller.
// FirstSeenAt is audit time, never admission expiry. Later bindings extend the
// receipt's FinalWriteAt; retention deletes bindings and receipt in one transaction.
type RuntimeKeyBinding struct {
	EnvironmentID  string    `gorm:"column:environment_id;primaryKey;not null"`
	IdempotencyKey string    `gorm:"column:idempotency_key;primaryKey;not null"`
	RunID          string    `gorm:"column:run_id;not null"`
	CallerID       string    `gorm:"column:caller_id;not null"`
	CallerName     *string   `gorm:"column:caller_name"`
	RequestDigest  string    `gorm:"column:request_digest;not null"`
	FirstSeenAt    time.Time `gorm:"column:first_seen_at;not null"`
}

func (RuntimeKeyBinding) TableName() string { return "runtime_key_bindings" }

// RuntimeStoreGeneration is inserted lazily by storage admission, never migration.
// Its random identity survives process restarts and activity/history cleanup.
type RuntimeStoreGeneration struct {
	ID         int       `gorm:"column:id;primaryKey;autoIncrement:false;not null"`
	Generation string    `gorm:"column:generation;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (RuntimeStoreGeneration) TableName() string { return "runtime_store_generation" }
