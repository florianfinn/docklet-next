// Source: getarcaneapp/arcane/types/event/stream.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package event

import "time"

// StreamEvent invalidates event queries after persistence changes.
type StreamEvent struct {
	// Type is "changed"; clients reload their current event query.
	Type string `json:"type"`
	// Timestamp is when the invalidation was emitted.
	Timestamp time.Time `json:"timestamp"`
}
