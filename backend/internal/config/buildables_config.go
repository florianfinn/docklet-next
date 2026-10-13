// Source: getarcaneapp/arcane/backend/internal/config/buildables_config.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build buildables

package config

// BuildablesConfig contains configuration for features that can be conditionally compiled.
// These fields are only available when the app is built with the buildables tag.
type BuildablesConfig struct {
	// Auto-login credentials (used only when built with the buildables tag + autologin feature flag).
	AutoLoginUsername string `env:"AUTO_LOGIN_USERNAME" default:"arcane"`
	AutoLoginPassword string `env:"AUTO_LOGIN_PASSWORD" default:"arcane-admin" options:"file"`
}
