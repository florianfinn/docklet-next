// Source: getarcaneapp/arcane/backend/internal/config/buildables_config_disabled.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build !buildables

package config

// BuildablesConfig provides stub fields when buildables are not enabled.
// This keeps references compile-safe while avoiding env-based configuration.
type BuildablesConfig struct {
	AutoLoginUsername string
	AutoLoginPassword string
}
