// Source: docklet-next/docs/design/hub-backup.md
// Revision: 73e62f9e35cd5ae59d1bb79802831f8204490866
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original regression coverage for resolved archive and database path separation.

package backup

import (
	"os"
	"path/filepath"
	"testing"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
)

func TestArchiveSeparationResolvesBothPaths(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	data := filepath.Join(real, "data")
	require.NoError(t, os.MkdirAll(data, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(data, "hub.db"), nil, 0600))
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(real, alias))
	for _, tc := range []struct {
		name, database, archive string
		allowed                 bool
	}{
		{"equal", "real/data", "real/data", false},
		{"archive-inside-data", "real/data", "real/data/archives", false},
		{"data-inside-archive", "real/data", "real", false},
		{"data-alias-equal", "alias/data", "real/data", false},
		{"data-alias-parent", "alias/data", "real", false},
		{"data-alias-child", "alias/data", "real/data/archives", false},
		{"archive-alias-equal", "real/data", "alias/data", false},
		{"archive-alias-child", "real/data", "alias/data/archives", false},
		{"archive-alias-parent", "real/data", "alias", false},
		{"siblings", "real/data", "real/archives", true},
		{"sibling-prefix", "real/data", "real/data-archives", true},
		{"separate-through-aliases", "alias/data", "alias/archives", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := filepath.Join(root, tc.archive)
			require.NoError(t, os.MkdirAll(archive, 0700))
			err := validateArchiveSeparation(archive, filepath.Join(root, tc.database, "hub.db"))
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrSystemBackupUnavailable)
			}
		})
	}
}

func TestNestedArchiveStorageUnavailableBeforeReconciliationOrSnapshot(t *testing.T) {
	db, cfg := archiveFixture(t)
	cfg.BackupDir = filepath.Join(filepath.Dir(db.Path()), "archives")
	require.NoError(t, os.Mkdir(cfg.BackupDir, 0700))
	canary := filepath.Join(cfg.BackupDir, ".partial-canary")
	require.NoError(t, os.WriteFile(canary, []byte("retain"), 0600))
	service := NewService(Dependencies{DB: db, Config: cfg})
	require.ErrorIs(t, service.databaseProviderError(), ErrSystemBackupUnavailable)
	status := service.Storage()
	require.False(t, status.Available)
	require.Equal(t, backuptypes.SystemBackupStorageMissing, status.ReasonCode)
	require.ErrorIs(t, service.ReconcileStorage(t.Context()), ErrSystemBackupUnavailable)
	_, _, err := service.snapshotArchive(t.Context(), "example", "example")
	require.ErrorIs(t, err, ErrSystemBackupUnavailable)
	require.False(t, service.MaintenanceGate().Status().Active)
	contents, err := os.ReadFile(canary)
	require.NoError(t, err)
	require.Equal(t, "retain", string(contents))
}
