// Source: getarcaneapp/arcane/types/lifecycle/lifecycle.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

// Package lifecycle contains lifecycle-hook values shared across Arcane modules.
package lifecycle

// ExtraMount is one configured bind mount for a lifecycle runner container.
type ExtraMount struct {
	Source   string
	Target   string
	Readonly bool
}

// DefaultTimeoutSec is the lifecycle timeout when a sync has no override.
const DefaultTimeoutSec = 60

// DefaultMaxTimeoutSec is the default lifecycle timeout ceiling.
const DefaultMaxTimeoutSec = 300
