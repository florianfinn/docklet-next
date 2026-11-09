// Source: docklet-next/issues/361
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original prerelease archive refusal and forward migration through the real restore receiver.

package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

// Preserve a real Francis image and build the hub image using the frozen SQL.
// Version 1 exercises identity refusal independently of the manifest version gate.
func prereleaseArchive(t *testing.T, version int64) []byte {
	t.Helper()
	source, cfg := archiveFixture(t)
	name, _, err := fixtureArchive(t, source, cfg, "template", 2<<30)
	require.NoError(t, err)
	file, err := os.Open(filepath.Join(cfg.BackupDir, name))
	require.NoError(t, err)
	defer file.Close()
	entries := map[string][]byte{}
	reader := tar.NewReader(file)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		entries[header.Name], err = io.ReadAll(reader)
		require.NoError(t, err)
	}
	var manifest archiveManifest
	require.NoError(t, json.Unmarshal(entries["manifest.json"], &manifest))
	path := filepath.Join(t.TempDir(), "prerelease.sqlite")
	pool, err := sql.Open("sqlite", sqlitestore.URL(path))
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, os.DirFS("../../../schema/testdata/prerelease-chain"))
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), version)
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	require.NoError(t, stampImage(t.Context(), path, manifest.SnapshotID, hubImageName))
	entries[hubImageName], err = os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(entries[hubImageName])
	manifest.HubSchema = version
	manifest.Images[0] = imageManifest{Name: hubImageName, Size: int64(len(entries[hubImageName])), SHA256: hex.EncodeToString(sum[:])}
	entries["manifest.json"], err = json.Marshal(manifest)
	require.NoError(t, err)
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	for _, name := range []string{"manifest.json", hubImageName, francisImageName} {
		data := entries[name]
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}))
		_, err = writer.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return output.Bytes()
}

func TestPrereleaseArchiveRefusedByUploadAndOfflineRestore(t *testing.T) {
	for _, version := range []int64{20, 1} {
		name := "m20-manifest"
		if version == 1 {
			name = "identity"
		}
		t.Run(name, func(t *testing.T) {
			archive := prereleaseArchive(t, version)
			target, cfg := archiveFixture(t)
			require.NoError(t, target.Exec("INSERT INTO settings(key,value) VALUES('destination-witness','unchanged')").Error)
			// Drain WAL so the witness hashes include all committed destination data.
			require.NoError(t, target.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error)
			paths := []string{target.Path(), sqlitestore.FrancisPath(target.Path())}
			before := make([][32]byte, len(paths))
			for i, path := range paths {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				before[i] = sha256.Sum256(data)
			}
			receiver := NewService(Dependencies{DB: target, Config: cfg})
			_, _, err := receiver.receiveArchive(t.Context(), "incoming", bytes.NewReader(archive))
			require.ErrorIs(t, err, errMigration)
			for i, path := range paths {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, before[i], sha256.Sum256(data))
			}
			require.NoError(t, target.Close())
			archiveName, err := backuptypes.ArchiveNameForID("incoming")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(cfg.BackupDir, archiveName), archive, 0600))
			require.ErrorIs(t, OfflineRestore(t.Context(), cfg, "incoming"), errMigration)
			for i, path := range paths {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, before[i], sha256.Sum256(data))
			}
			require.NoError(t, CheckPendingRestore(cfg))
		})
	}
}

func forwardRestoreSource(t *testing.T) fs.FS {
	t.Helper()
	data, err := resources.FS.ReadFile("migrations/sqlite/001_baseline.sql")
	require.NoError(t, err)
	return fstest.MapFS{
		"001_baseline.sql": &fstest.MapFile{Data: data},
		"002_fixture.sql":  &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE forward_restore_witness(value TEXT);\nINSERT INTO forward_restore_witness SELECT value FROM settings WHERE key='forward-restore';\n")},
	}
}

func TestRestoreAppliesForwardMigrationFromProcessSource(t *testing.T) {
	source := restoreArchiveNew(t)
	require.NoError(t, source.db.Exec("INSERT INTO settings(key,value) VALUES('forward-restore','saved-from-archive')").Error)
	record := restoreArchiveMetadata(t, source)
	options := database.MigrationOptions{Source: forwardRestoreSource(t)}
	target := restoreArchiveNew(t, options)
	target.cfg.BackupDir = source.cfg.BackupDir
	saved := false
	service := NewService(Dependencies{DB: target.db, Config: target.cfg, Activity: target.activities, Store: Store{
		Run:     func(context.Context, string) (*backuptypes.SystemBackupRun, error) { return record, nil },
		SaveRun: func(context.Context, *backuptypes.SystemBackupRun) error { saved = true; return nil },
	}})
	unready := false
	service.SetExitUnready(func() { unready = true })
	workCtx, started := restoreArchiveStart(t, service, target, record)
	ctx, cancel := context.WithTimeout(workCtx, 20*time.Second)
	defer cancel()
	service.executeRestore(ctx, usertypes.Actor{ID: "example-restorer", Username: "Restore operator"}, "", record.ID, started)
	require.False(t, unready)
	require.True(t, saved)
	require.NoError(t, CheckPendingRestore(target.cfg))
	var value string
	require.NoError(t, target.db.Raw("SELECT value FROM forward_restore_witness").Scan(&value).Error)
	require.Equal(t, "saved-from-archive", value)
	var version int64
	require.NoError(t, target.db.Raw("SELECT max(version_id) FROM goose_db_version").Scan(&version).Error)
	require.EqualValues(t, 2, version)
	// Newer supported archives must use the process source at the upload receiver too.
	name, _, err := fixtureArchive(t, target.db, target.cfg, "forward", 2<<30)
	require.NoError(t, err)
	file, err := os.Open(filepath.Join(target.cfg.BackupDir, name))
	require.NoError(t, err)
	defer file.Close()
	_, info, err := service.receiveArchive(t.Context(), "forward-upload", file)
	require.NoError(t, err)
	require.EqualValues(t, 2, info.migration)
}

func TestOfflineRestoreRefusesPrereleaseDestinationWithoutWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "destination.db")
	pool, err := sql.Open("sqlite", sqlitestore.URL(path))
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, os.DirFS("../../../schema/testdata/prerelease-chain"))
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	require.NoError(t, os.Chmod(path, 0640))
	sidecar := sqlitestore.FrancisPath(path)
	require.NoError(t, francis.InitializeStore(t.Context(), sqlitestore.URL(path)))
	require.NoError(t, os.Chmod(sidecar, 0640))
	type state struct {
		Exists bool
		Hash   [32]byte
		Mode   fs.FileMode
	}
	read := func(name string) state {
		info, err := os.Stat(name)
		if os.IsNotExist(err) {
			return state{}
		}
		require.NoError(t, err)
		data, err := os.ReadFile(name)
		require.NoError(t, err)
		return state{true, sha256.Sum256(data), info.Mode().Perm()}
	}
	paths := []string{path, path + "-wal", path + "-shm", sidecar, sidecar + "-wal", sidecar + "-shm"}
	before := make([]state, len(paths))
	for i, name := range paths {
		before[i] = read(name)
	}
	cfg := &config.Config{DatabaseURL: sqlitestore.URL(path), BackupDir: t.TempDir()}
	err = OfflineRestore(t.Context(), cfg, "missing")
	for i, name := range paths {
		require.Equal(t, before[i], read(name), filepath.Base(name))
	}
	require.ErrorIs(t, err, errMigration)
}
