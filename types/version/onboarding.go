// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package version

// OnboardingChecks describes checks made with the agent's effective identity.
// Nil fields mean the check could not be established, never a successful check.
type OnboardingChecks struct {
	ProjectDirectory string  `json:"projectDirectory"`
	OwnerUID         *uint32 `json:"ownerUid"`
	OwnerGID         *uint32 `json:"ownerGid"`
	Writable         *bool   `json:"writable"`
	SocketGroup      *uint32 `json:"socketGroup"`
}
