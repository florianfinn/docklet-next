// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package user

// AppAssignment grants access to an agent-local project UUID on one host.
type AppAssignment struct {
	EnvironmentID string `json:"environmentId" minLength:"1"`
	ProjectID     string `json:"projectId" minLength:"1"`
	Level         string `json:"level" enum:"restricted,full"`
}
