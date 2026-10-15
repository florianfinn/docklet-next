// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38.
package capability

type ResourceRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Descriptor struct {
	OperationID        string      `json:"operationId"`
	Available          bool        `json:"available"`
	DisabledReasonCode string      `json:"disabledReasonCode,omitempty"`
	ResourceRef        ResourceRef `json:"resourceRef"`
}
