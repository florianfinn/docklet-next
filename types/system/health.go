// Source: getarcaneapp/arcane/types/system/health.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None; M14 container and host operation contracts.

package system

// HealthResponse contains the health status of the API.
type HealthResponse struct {
	InstanceID string `json:"instanceId"`
	// Status indicates the health status (e.g., "UP", "DOWN").
	//
	// Required: true
	Status string `json:"status"`
}
