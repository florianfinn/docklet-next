// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original startup ordering regression test for issue #32.

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	hubbackup "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
)

func TestBootstrapRefusesRestoreMarkerBeforeDatabaseMigration(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "restore-state.json"), []byte("marker"), 0600))
	t.Setenv("DOCKLET_BACKUP_DIR", dir)
	t.Setenv("DOCKER_CONFIG", filepath.Join(dir, "docker-config"))
	t.Setenv("PROJECTS_DIRECTORY", filepath.Join(dir, "projects"))
	t.Setenv("DATABASE_URL", "postgres://localhost:1/ignored")
	err := Bootstrap(t.Context())
	require.ErrorIs(t, err, hubbackup.ErrRestoreRecoveryRequired)
}
