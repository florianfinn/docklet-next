// Source: getarcaneapp/arcane/types/environment/swarm.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package environment

// SwarmBindingCandidate is the binding-relevant state of an environment being attached to a node.
type SwarmBindingCandidate struct {
	Hidden              bool
	Enabled             bool
	ParentEnvironmentID *string
	SwarmNodeID         *string
}
