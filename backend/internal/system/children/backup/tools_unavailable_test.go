// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package backup

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteBackupRequiresStorageWithoutExternalTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	service := &Service{config: &config.Config{DatabaseURL: "file:example.db", BackupDir: t.TempDir()}}
	require.True(t, service.Storage().Available)
	blocked := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0600))
	service.config.BackupDir = filepath.Join(blocked, "backups")
	require.False(t, service.Storage().Available)
	require.Equal(t, backuptypes.SystemBackupStorageMissing, service.Storage().ReasonCode)
}
