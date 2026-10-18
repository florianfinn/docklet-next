// Source: docklet-next/docs/design/runtime-control.md
// Revision: 687e1abd5ed1be0e69a7fdc4c4677da2db78b85d
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant runtime projection vocabulary, separate from registered v3 DTOs.

package activity

// RuntimeOutcomeState describes receipt knowledge, independently of activity status.
// Known describes workflow disposition, not verified Docker state or rollback.
type RuntimeOutcomeState string

const (
	RuntimeOutcomeKnown   RuntimeOutcomeState = "known"
	RuntimeOutcomeUnknown RuntimeOutcomeState = "unknown"
)

// RuntimeKnowledge is required as a pair on runtime projections. Historical and
// unrelated Activity values carry neither field; cancellation intent is not outcome.
type RuntimeKnowledge struct {
	OutcomeState    RuntimeOutcomeState `json:"outcomeState"`
	CancelRequested bool                `json:"cancelRequested"`
}

// RuntimeSnapshot adds flat receipt knowledge without changing registered Activity.
// A hub release may project failed while executor outcome remains unknown.
// This standalone type is not registered with the public API before activation.
type RuntimeSnapshot struct {
	Activity
	RuntimeKnowledge
}

// RuntimeAdmission confirms acceptance, never completed success. Code reuses
// CodeRunStarted or CodeRunJoined; ActivityID and Activity.ID identify the same run.
type RuntimeAdmission struct {
	ActivityID string          `json:"activityId"`
	Code       Code            `json:"code"`
	Activity   RuntimeSnapshot `json:"activity"`
}

// These dormant codes are deliberately outside Values and registered v3 enums.
const (
	CodeRunRequestMismatch        Code = "run.request-mismatch"
	CodeRuntimeOutcomeUnknown     Code = "runtime.outcome-unknown"
	CodeRuntimeInterrupted        Code = "runtime.interrupted"
	CodeRuntimeCancelled          Code = "runtime.cancelled"
	CodeRuntimeFailed             Code = "runtime.failed"
	CodeRuntimeWaitTimeout        Code = "runtime.wait-timeout"
	CodeRuntimeWaiterDisconnected Code = "runtime.waiter-disconnected"
	CodeRuntimeParentUnresolved   Code = "runtime.parent-unresolved"
	CodeRuntimeTargetRemoved      Code = "runtime.target-removed"
)
