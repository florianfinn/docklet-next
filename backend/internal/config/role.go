// Source: docklet-next/issues/299
// Revision: 8696c990fba68be0dd3ca9ac4de61434f6c56131
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original side-effect-free process role reader.

package config

// Role is the process role subset read on hot and concurrent paths.
type Role struct {
	AgentMode               bool
	EdgeAgent               bool
	UIConfigurationDisabled bool
	Environment             string
}

// LoadRole reads the role with the same presence rules as Load, without writing package globals.
func LoadRole() Role {
	cfg := &Config{}
	loadFromEnv(cfg)
	applyAgentModeDefaults(cfg)
	return Role{AgentMode: cfg.AgentMode, EdgeAgent: cfg.EdgeAgent, UIConfigurationDisabled: cfg.UIConfigurationDisabled, Environment: string(cfg.Environment)}
}
