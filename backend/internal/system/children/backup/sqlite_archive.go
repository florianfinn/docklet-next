// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package backup

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

const archiveVersion = 1
const francisSchemaVersion = 7
const hubImageName = "hub.sqlite"
const francisImageName = "francis.sqlite"

type imageManifest struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type archiveManifest struct {
	SnapshotID    string          `json:"snapshotId"`
	Format        string          `json:"format"`
	Version       int             `json:"version"`
	HubSchema     int64           `json:"hubSchema"`
	FrancisSchema int             `json:"francisSchema"`
	Images        []imageManifest `json:"images"`
}

// Sources are reserved writer connections, with admissions and transactions
// drained for both files before either VACUUM starts. SQLite includes all WAL data.
func createSQLiteArchive(ctx, snapshotCtx context.Context, dir, id string, hub, francis *sql.Conn, limit int64, release ...func()) (string, int64, error) {
	if err := archiveContextError(ctx); err != nil {
		return "", 0, err
	}
	name, err := backuptypes.ArchiveNameForID(id)
	if err != nil {
		return "", 0, errInvalidArchive
	}
	if err = storageAvailable(dir); err != nil {
		return "", 0, err
	}
	var estimated int64
	for _, conn := range []*sql.Conn{hub, francis} {
		var pages, pageSize int64
		if conn.QueryRowContext(snapshotCtx, "PRAGMA page_count").Scan(&pages) != nil || conn.QueryRowContext(snapshotCtx, "PRAGMA page_size").Scan(&pageSize) != nil {
			return "", 0, archiveValidationError(snapshotCtx)
		}
		if pages < 1 || pageSize < 1 || pages > (backuptypes.MaxSystemArchiveBytes-estimated)/pageSize {
			return "", 0, errArchiveLimit
		}
		estimated += pages * pageSize
	}
	if estimated+16384 > limit {
		return "", 0, errArchiveLimit
	}
	if err := requireFreeSpace(dir, 3*estimated); err != nil {
		return "", 0, err
	}
	stage, err := os.MkdirTemp(dir, ".snapshot-")
	if err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	defer os.RemoveAll(stage)
	for i, conn := range []*sql.Conn{hub, francis} {
		image := []string{hubImageName, francisImageName}[i]
		if _, err = conn.ExecContext(snapshotCtx, "VACUUM INTO ?", filepath.Join(stage, image)); err != nil {
			return "", 0, archiveValidationError(snapshotCtx)
		}
		if err = os.Chmod(filepath.Join(stage, image), 0600); err != nil {
			return "", 0, ErrSystemBackupUnavailable
		}
		if err = syncFile(filepath.Join(stage, image)); err != nil {
			return "", 0, ErrSystemBackupUnavailable
		}
	}
	if err := archiveContextError(snapshotCtx); err != nil {
		return "", 0, err
	}
	if len(release) > 0 {
		release[0]()
	}
	snapshotID := uuid.New().String()
	for _, name := range []string{hubImageName, francisImageName} {
		if err := stampImage(ctx, filepath.Join(stage, name), snapshotID, name); err != nil {
			return "", 0, err
		}
	}
	newest, err := newestMigration()
	if err != nil {
		return "", 0, errMigration
	}
	version, err := validateImages(ctx, stage, newest)
	if err != nil {
		return "", 0, err
	}
	manifest := archiveManifest{SnapshotID: snapshotID, Format: "docklet-sqlite", Version: archiveVersion, HubSchema: version, FrancisSchema: francisSchemaVersion}
	for _, image := range []string{hubImageName, francisImageName} {
		item, err := describeImage(ctx, filepath.Join(stage, image), image)
		if err != nil {
			return "", 0, err
		}
		manifest.Images = append(manifest.Images, item)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", 0, errInvalidArchive
	}
	file, err := os.CreateTemp(dir, ".partial-")
	if err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	defer os.Remove(file.Name())
	defer file.Close()
	writer := &limitedArchiveWriter{file: file, limit: limit}
	tw := tar.NewWriter(writer)
	if err = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err == nil {
		_, err = tw.Write(data)
	}
	if err != nil {
		return "", 0, err
	}
	for _, image := range manifest.Images {
		if err = tw.WriteHeader(&tar.Header{Name: image.Name, Mode: 0600, Size: image.Size, Typeflag: tar.TypeReg}); err != nil {
			return "", 0, err
		}
		source, err := os.Open(filepath.Join(stage, image.Name))
		if err != nil {
			return "", 0, errInvalidArchive
		}
		_, err = io.Copy(tw, &contextReader{ctx: ctx, reader: source})
		closeErr := source.Close()
		if err = errors.Join(err, closeErr); err != nil {
			return "", 0, err
		}
	}
	if err = tw.Close(); err != nil {
		return "", 0, err
	}
	if err = file.Sync(); err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	if err = file.Close(); err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	// Re-read the finished tar rather than publishing based only on the inputs.
	if _, err = validateArchive(ctx, file.Name(), newest); err != nil {
		return "", 0, err
	}
	if err = os.Rename(file.Name(), filepath.Join(dir, name)); err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	if err = syncArchiveDir(dir); err != nil {
		return "", 0, err
	}
	return name, writer.written, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := archiveContextError(r.ctx); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func describeImage(ctx context.Context, path, name string) (imageManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return imageManifest{}, errInvalidArchive
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, &contextReader{ctx: ctx, reader: file})
	if err != nil {
		return imageManifest{}, err
	}
	return imageManifest{Name: name, Size: n, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}
func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}
func openImage(path string) (*sql.DB, error) {
	u, _ := url.Parse(sqlitestore.URL(path))
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("immutable", "1")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	pool, err := sql.Open("sqlite", u.String())
	if err == nil {
		pool.SetMaxOpenConns(1)
	}
	return pool, err
}
func checkImage(ctx context.Context, pool *sql.DB) error {
	rows, err := pool.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return errInvalidArchive
	}
	count := 0
	for rows.Next() {
		var result string
		if rows.Scan(&result) != nil || result != "ok" {
			_ = rows.Close()
			return errInvalidArchive
		}
		count++
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil || count != 1 {
		return errInvalidArchive
	}
	rows, err = pool.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return errInvalidArchive
	}
	bad := rows.Next()
	err = errors.Join(rows.Err(), rows.Close())
	if bad || err != nil {
		return errInvalidArchive
	}
	return nil
}
func validateImages(ctx context.Context, dir string, newest int64) (int64, error) {
	hub, err := openImage(filepath.Join(dir, hubImageName))
	if err != nil {
		return 0, errInvalidArchive
	}
	defer hub.Close()
	if err = checkImage(ctx, hub); err != nil {
		return 0, err
	}
	if err = database.ValidateSnapshot(ctx, hub, newest); err != nil {
		return 0, errMigration
	}
	var version int64
	if err = hub.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version WHERE is_applied=1").Scan(&version); err != nil {
		return 0, errMigration
	}
	sidecar, err := openImage(filepath.Join(dir, francisImageName))
	if err != nil {
		return 0, errInvalidArchive
	}
	defer sidecar.Close()
	if err = checkImage(ctx, sidecar); err != nil {
		return 0, err
	}
	var francisVersion int
	if err = sidecar.QueryRowContext(ctx, "SELECT value FROM arcane_francis_metadata WHERE key='migrations-version'").Scan(&francisVersion); err != nil || francisVersion != francisSchemaVersion {
		return 0, errMigration
	}
	// Check the expected namespace, including the ownership tables cleared on restore.
	for _, table := range []string{"hosts", "host_actor_types", "active_actors", "actor_state", "alarms", "cluster_config", "terminal_jobs"} {
		var n int
		if err = sidecar.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", "arcane_francis_"+table).Scan(&n); err != nil || n != 1 {
			return 0, errInvalidArchive
		}
	}
	hubID, err := imageSnapshotID(ctx, hub, hubImageName)
	if err != nil {
		return 0, err
	}
	sidecarID, err := imageSnapshotID(ctx, sidecar, francisImageName)
	if err != nil || hubID != sidecarID {
		return 0, errInvalidArchive
	}
	return version, nil
}

func stampImage(ctx context.Context, path, id, role string) error {
	pool, err := sql.Open("sqlite", sqlitestore.URL(path))
	if err != nil {
		return errInvalidArchive
	}
	defer pool.Close()
	if _, err = pool.ExecContext(ctx, "DROP TABLE IF EXISTS docklet_backup_image"); err != nil {
		return errInvalidArchive
	}
	if _, err = pool.ExecContext(ctx, "CREATE TABLE docklet_backup_image(snapshot_id TEXT NOT NULL, role TEXT NOT NULL)"); err != nil {
		return errInvalidArchive
	}
	if _, err = pool.ExecContext(ctx, "INSERT INTO docklet_backup_image(snapshot_id,role) VALUES(?,?)", id, role); err != nil {
		return errInvalidArchive
	}
	if err = pool.Close(); err != nil {
		return errInvalidArchive
	}
	return syncFile(path)
}
func imageSnapshotID(ctx context.Context, pool *sql.DB, role string) (string, error) {
	var id, storedRole string
	var count int
	if err := pool.QueryRowContext(ctx, "SELECT count(*),coalesce(max(snapshot_id),''),coalesce(max(role),'') FROM docklet_backup_image").Scan(&count, &id, &storedRole); err != nil || count != 1 || storedRole != role {
		return "", errInvalidArchive
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", errInvalidArchive
	}
	return id, nil
}
