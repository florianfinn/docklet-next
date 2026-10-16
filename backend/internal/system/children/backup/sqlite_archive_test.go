// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package backup

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

func archiveFixture(t *testing.T) (*database.DB, *config.Config) {
	t.Helper()
	cfg := &config.Config{DatabaseURL: sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), BackupDir: t.TempDir()}
	db, err := database.InitializeHub(t.Context(), cfg.DatabaseURL, database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, francis.InitializeStore(t.Context(), cfg.DatabaseURL))
	return db, cfg
}
func fixtureArchive(t *testing.T, db *database.DB, cfg *config.Config, id string, limit int64) (string, int64, error) {
	t.Helper()
	pool, err := db.SQLDB()
	require.NoError(t, err)
	hub, err := pool.Conn(t.Context())
	require.NoError(t, err)
	defer hub.Close()
	sidecar, err := sqlitestore.Open(t.Context(), sqlitestore.FrancisPath(db.Path()), false, false)
	require.NoError(t, err)
	defer sidecar.Close()
	conn, err := sidecar.Conn(t.Context())
	require.NoError(t, err)
	defer conn.Close()
	return createSQLiteArchive(t.Context(), cfg.BackupDir, id, hub, conn, limit)
}
func TestSQLiteSnapshotIncludesCommittedWALAndValidatesPair(t *testing.T) {
	db, cfg := archiveFixture(t)
	require.NoError(t, db.Exec("INSERT INTO settings(key,value) VALUES('snapshot-example','retained')").Error)
	name, _, err := fixtureArchive(t, db, cfg, "wal", 2<<30)
	require.NoError(t, err)
	stage := t.TempDir()
	info, err := extractArchive(t.Context(), filepath.Join(cfg.BackupDir, name), stage, mustNewestMigration())
	require.NoError(t, err)
	require.Equal(t, mustNewestMigration(), info.migration)
	image, err := openImage(filepath.Join(stage, hubImageName))
	require.NoError(t, err)
	defer image.Close()
	var value string
	require.NoError(t, image.QueryRow("SELECT value FROM settings WHERE key='snapshot-example'").Scan(&value))
	require.Equal(t, "retained", value)
}
func TestSQLiteSnapshotBarrier(t *testing.T) {
	db, cfg := archiveFixture(t)
	svc := NewService(Dependencies{DB: db, Config: cfg, Activity: activity.NewActivityService(db, nil)})
	_, _, err := svc.snapshotArchive(t.Context(), "backup-activity", "barrier")
	require.NoError(t, err)
	require.False(t, svc.MaintenanceGate().Status().Active)
}
func TestSQLiteArchiveLimitRemovesIncompleteFiles(t *testing.T) {
	db, cfg := archiveFixture(t)
	_, _, err := fixtureArchive(t, db, cfg, "limited", 1024)
	require.ErrorIs(t, err, errArchiveLimit)
	entries, err := os.ReadDir(cfg.BackupDir)
	require.NoError(t, err)
	require.Empty(t, entries)
}
func rewriteTar(t *testing.T, source, target string, change func(*tar.Header, []byte) (*tar.Header, []byte)) {
	t.Helper()
	in, err := os.Open(source)
	require.NoError(t, err)
	defer in.Close()
	out, err := os.Create(target)
	require.NoError(t, err)
	defer out.Close()
	tr := tar.NewReader(in)
	tw := tar.NewWriter(out)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(tr)
		require.NoError(t, err)
		h, data = change(h, data)
		h.Size = int64(len(data))
		require.NoError(t, tw.WriteHeader(h))
		_, err = tw.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
}
func TestSQLiteArchiveRejectsCorruptionMismatchVersionsAndPaths(t *testing.T) {
	db, cfg := archiveFixture(t)
	name, _, err := fixtureArchive(t, db, cfg, "valid", 2<<30)
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		want   error
		change func(*tar.Header, []byte) (*tar.Header, []byte)
	}{
		{"hash", errInvalidArchive, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
			if h.Name == hubImageName {
				b[len(b)-1] ^= 1
			}
			return h, b
		}},
		{"path", errInvalidArchive, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
			if h.Name == hubImageName {
				h.Name = "../outside"
			}
			return h, b
		}},
		{"link", errInvalidArchive, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
			if h.Name == hubImageName {
				h.Typeflag = tar.TypeSymlink
				h.Linkname = "outside"
				b = nil
			}
			return h, b
		}},
		{"format", errArchiveMajor, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
			if h.Name == "manifest.json" {
				var m archiveManifest
				require.NoError(t, json.Unmarshal(b, &m))
				m.Version++
				b, _ = json.Marshal(m)
			}
			return h, b
		}},
		{"newer", errMigration, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
			if h.Name == "manifest.json" {
				var m archiveManifest
				require.NoError(t, json.Unmarshal(b, &m))
				m.HubSchema++
				b, _ = json.Marshal(m)
			}
			return h, b
		}},
		{"francis", errMigration, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
			if h.Name == "manifest.json" {
				var m archiveManifest
				require.NoError(t, json.Unmarshal(b, &m))
				m.FrancisSchema++
				b, _ = json.Marshal(m)
			}
			return h, b
		}},
		{"mismatched", errInvalidArchive, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
			if h.Name == "manifest.json" {
				var m archiveManifest
				require.NoError(t, json.Unmarshal(b, &m))
				m.Images[0].SHA256 = m.Images[1].SHA256
				b, _ = json.Marshal(m)
			}
			return h, b
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "invalid.tar")
			rewriteTar(t, filepath.Join(cfg.BackupDir, name), target, tc.change)
			_, err := validateArchive(t.Context(), target, mustNewestMigration())
			require.ErrorIs(t, err, tc.want)
		})
	}
}
func TestSQLiteSnapshotChecksIntegrityIdentityAndForeignKeys(t *testing.T) {
	for _, query := range []string{
		"DROP TABLE docklet_database_metadata",
		"INSERT INTO goose_db_version(version_id,is_applied) VALUES(999,1)",
		"PRAGMA foreign_keys=OFF;INSERT INTO user_sessions(id,created_at,user_id,refresh_token_hash,last_used_at,expires_at) VALUES('invalid',1,'missing','hash',1,2)",
	} {
		t.Run(query[:8], func(t *testing.T) {
			db, cfg := archiveFixture(t)
			require.NoError(t, db.Exec(query).Error)
			_, _, err := fixtureArchive(t, db, cfg, "invalid", 2<<30)
			require.Error(t, err)
			_, err = os.Stat(filepath.Join(cfg.BackupDir, "invalid.tar"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
func TestSQLiteUploadAcceptsPairWithoutExternalTools(t *testing.T) {
	db, cfg := archiveFixture(t)
	name, _, err := fixtureArchive(t, db, cfg, "source", 2<<30)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(cfg.BackupDir, name))
	require.NoError(t, err)
	t.Setenv("PATH", t.TempDir())
	svc := &Service{config: cfg}
	uploaded, info, err := svc.receiveArchive(t.Context(), "uploaded", bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, "uploaded.tar", uploaded)
	require.Positive(t, info.size)
}

func TestSQLiteArchiveRejectsDifferentValidPairEvenWithRecomputedHash(t *testing.T) {
	db, cfg := archiveFixture(t)
	first, _, err := fixtureArchive(t, db, cfg, "first", 2<<30)
	require.NoError(t, err)
	second, _, err := fixtureArchive(t, db, cfg, "second", 2<<30)
	require.NoError(t, err)
	stage := t.TempDir()
	_, err = extractArchive(t.Context(), filepath.Join(cfg.BackupDir, second), stage, mustNewestMigration())
	require.NoError(t, err)
	replacement, err := os.ReadFile(filepath.Join(stage, francisImageName))
	require.NoError(t, err)
	description, err := describeImage(t.Context(), filepath.Join(stage, francisImageName), francisImageName)
	require.NoError(t, err)
	target := filepath.Join(t.TempDir(), "mixed.tar")
	rewriteTar(t, filepath.Join(cfg.BackupDir, first), target, func(h *tar.Header, data []byte) (*tar.Header, []byte) {
		if h.Name == "manifest.json" {
			var m archiveManifest
			require.NoError(t, json.Unmarshal(data, &m))
			m.Images[1] = description
			data, _ = json.Marshal(m)
		}
		if h.Name == francisImageName {
			data = replacement
		}
		return h, data
	})
	_, err = validateArchive(t.Context(), target, mustNewestMigration())
	require.ErrorIs(t, err, errInvalidArchive)
}
