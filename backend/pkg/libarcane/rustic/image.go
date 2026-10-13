// Source: getarcaneapp/arcane/backend/pkg/libarcane/rustic/image.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

// Package rustic provides shared Rustic runtime configuration.
package rustic

const (
	// DefaultImage is the official Rustic container image used by backup and recovery flows.
	DefaultImage = "ghcr.io/rustic-rs/rustic:v0.11.2"

	// CacheVolume persists Rustic's repository cache across ephemeral runtime containers.
	CacheVolume = "arcane-rustic-cache"
)
