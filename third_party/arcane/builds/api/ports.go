// Source: getarcaneapp/kit/builds/api/ports.go
// Revision: 83d91f19f60b2967cc85c46161c98203e80e4989
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package api

import (
	"log/slog"

	"go.getarcane.app/builds/types"
)

// SettingsProvider provides build settings owned by the host application.
type SettingsProvider = types.SettingsProvider

// DockerClientProvider provides Docker clients.
type DockerClientProvider = types.DockerClientProvider

// RegistryAuthProvider provides registry credentials owned by the host application.
type RegistryAuthProvider = types.RegistryAuthProvider

// Config configures Service.
type Config struct {
	SettingsProvider     SettingsProvider
	DockerClientProvider DockerClientProvider
	RegistryAuthProvider RegistryAuthProvider
	Logger               *slog.Logger
}
