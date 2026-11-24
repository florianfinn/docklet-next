// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation. M38 #601: agent effective IDs.

package version

// OnboardingChecks describes checks made with the agent's effective identity.
// Nil fields mean the check could not be established, never a successful check.
type OnboardingChecks struct {
	ProjectDirectory string  `json:"projectDirectory"`
	OwnerUID         *uint32 `json:"ownerUid"`
	OwnerGID         *uint32 `json:"ownerGid"`
	Writable         *bool   `json:"writable"`
	SocketGroup      *uint32 `json:"socketGroup"`
	// AgentUID and AgentGID are the agent's effective IDs (#601). Absent from older agents: unknown, never root.
	// Pointers with omitzero keep 0 (root) on the wire and omit only an unknown value.
	AgentUID *uint32 `json:"agentUid,omitzero"`
	AgentGID *uint32 `json:"agentGid,omitzero"`
}
