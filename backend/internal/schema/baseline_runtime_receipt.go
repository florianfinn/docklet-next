// Source: docklet-next/issues/531
// Revision: 060f8d107d05717e605964030d6c52bedcab7fb8
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original frozen copy of the runtime receipt model as shipped in migration 001.

package schema

import "time"

// baselineRuntimeReceipt is the receipt model exactly as migration 001 created it.
// Columns added by later forward migrations (006) stay on activity.RuntimeReceipt only.
type baselineRuntimeReceipt struct {
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
	Result               *string    `gorm:"column:result;type:text"`
}

func (baselineRuntimeReceipt) TableName() string { return "runtime_receipts" }
