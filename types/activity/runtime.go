// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Required runtime projection vocabulary for activated v4 DTOs.

package activity

import "github.com/florianfinn/docklet-next/types/container"

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
	OutcomeState    RuntimeOutcomeState `json:"outcomeState" enum:"known,unknown"`
	CancelRequested bool                `json:"cancelRequested"`
}

// RuntimeSnapshot requires flat receipt knowledge beside the activity projection.
// A hub release may project failed while executor outcome remains unknown.
type RuntimeSnapshot struct {
	Result *container.RecreateResult `json:"result,omitempty"`
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

// Runtime codes are part of the activated v4 lifecycle vocabulary.
const (
	CodeRuntimeTargetChanged      Code = "runtime.target-changed"
	CodeRuntimeTargetUnavailable  Code = "runtime.target-unavailable"
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
