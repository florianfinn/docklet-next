// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Hub recovery evidence remains separate from executor disposition.

package activity

import "time"

type RuntimeRecoveryEvidence struct {
	UnknownReason       *string    `json:"unknownReason,omitempty"`
	ExecutorState       string     `json:"executorState"`
	ExecutorResultCode  *string    `json:"executorResultCode,omitempty"`
	ExecutorError       *string    `json:"executorError,omitempty"`
	ExecutorSettledAt   *time.Time `json:"executorSettledAt,omitempty"`
	Disposition         *string    `json:"disposition,omitempty"`
	DispositionBy       *string    `json:"dispositionBy,omitempty"`
	DispositionReason   *string    `json:"dispositionReason,omitempty"`
	DispositionAt       *time.Time `json:"dispositionAt,omitempty"`
	ReleaseAcknowledged bool       `json:"releaseAcknowledged"`
}

type RuntimeHubDetail struct {
	RuntimeExecutionDetail
	Recovery RuntimeRecoveryEvidence `json:"recovery"`
}
