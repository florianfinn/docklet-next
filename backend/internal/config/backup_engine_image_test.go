// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original per-process engine image environment configuration proof.

package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBackupEngineImageEnvironmentOverride(t *testing.T) {
	t.Setenv("DOCKLET_BACKUP_ENGINE_IMAGE", "")
	require.Empty(t, Load().BackupEngineImage)
	t.Setenv("DOCKLET_BACKUP_ENGINE_IMAGE", "registry.example.com/rustic:0.11.2")
	require.Equal(t, "registry.example.com/rustic:0.11.2", Load().BackupEngineImage)
}
