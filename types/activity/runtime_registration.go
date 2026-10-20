// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Manager-authenticated executor registration and fresh target metadata.

package activity

// RuntimeRegistration is constructed from the hub's persisted instance and host
// registration. Possession of these public identifiers grants no authority.
type RuntimeRegistration struct {
	ManagerID      string `json:"managerId"`
	RegistrationID string `json:"registrationId"`
}

type RuntimeTargetRequest struct {
	Registration RuntimeExecutorIdentity `json:"registration"`
	Target       RuntimeTarget           `json:"target"`
}

type RuntimeTargetMetadata struct {
	Target         RuntimeTarget `json:"target"`
	ComposeManaged bool          `json:"composeManaged"`
}
