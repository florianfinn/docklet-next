// Source: docklet-next/issues/570
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original new-name and deprecated-alias precedence proofs.; M36 #570: Docklet backup volume and runtime names.

package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBackupVolumeEnvironmentAliases(t *testing.T) {
	for _, tc := range []struct{ name, current, legacy, want string }{
		{"unset", "", "", ""},
		{"new", "new-backups", "", "new-backups"},
		{"alias", "", "old-backups", "old-backups"},
		{"both", "new-backups", "old-backups", "new-backups"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DOCKLET_BACKUP_VOLUME_NAME", tc.current)
			t.Setenv("ARCANE_BACKUP_VOLUME_NAME", tc.legacy)
			cfg := Load()
			require.Equal(t, tc.want, cfg.BackupVolumeName)
			count := 0
			for _, name := range cfg.DeprecatedEnvVarsSet() {
				if name == "ARCANE_BACKUP_VOLUME_NAME" {
					count++
				}
			}
			if tc.legacy != "" {
				require.Equal(t, 1, count)
			} else {
				require.Zero(t, count)
			}
		})
	}
}
