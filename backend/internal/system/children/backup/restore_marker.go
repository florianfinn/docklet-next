// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

const restoreMarkerName = "restore-state.json"

var ErrRestoreRecoveryRequired = errors.New("backup.recovery_required")

type restoreMarker struct {
	ArchiveSize      int64           `json:"archiveSize"`
	MigrationVersion int64           `json:"migrationVersion"`
	Version          int             `json:"version"`
	BackupID         string          `json:"backupId"`
	RestoreID        string          `json:"restoreId"`
	ActivityID       string          `json:"activityId,omitempty"`
	Phase            string          `json:"phase"`
	DatabasePath     string          `json:"databasePath"`
	Images           []imageManifest `json:"images"`
}

func restoreStateDir(path string) string { return path + ".restore" }

func CheckPendingRestore(cfg *config.Config) error {
	path, err := sqlitestore.Path(cfg.DatabaseURL)
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	return CheckRestoreMarker(restoreStateDir(path))
}

func markerPath(dir string) string { return filepath.Join(dir, restoreMarkerName) }
func CheckRestoreMarker(dir string) error {
	parent, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return ErrRestoreRecoveryRequired
	}
	_, err = os.Lstat(markerPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return ErrRestoreRecoveryRequired
}
func readRestoreMarker(dir string) (restoreMarker, error) {
	var marker restoreMarker
	info, err := os.Lstat(markerPath(dir))
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return marker, ErrRestoreRecoveryRequired
	}
	data, err := os.ReadFile(markerPath(dir))
	if err != nil || json.Unmarshal(data, &marker) != nil {
		return marker, ErrRestoreRecoveryRequired
	}
	if _, err = backuptypes.ArchiveNameForID(marker.RestoreID); err != nil {
		return marker, ErrRestoreRecoveryRequired
	}
	if _, err = backuptypes.ArchiveNameForID(marker.BackupID); err != nil || marker.Version != 1 || len(marker.Images) != 2 || marker.ArchiveSize < 1024 || marker.ArchiveSize > backuptypes.MaxSystemArchiveBytes || marker.MigrationVersion < 1 || !filepath.IsAbs(marker.DatabasePath) || (marker.Phase != "applying" && marker.Phase != "verifying") {
		return restoreMarker{}, ErrRestoreRecoveryRequired
	}
	for i, name := range []string{hubImageName, francisImageName} {
		item := marker.Images[i]
		if item.Name != name || len(item.SHA256) != 64 || item.Size < 512 {
			return restoreMarker{}, ErrRestoreRecoveryRequired
		}
	}
	return marker, nil
}
func writeRestoreMarker(dir string, marker restoreMarker) error {
	if err := ensureArchiveDir(dir); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := syncArchiveDir(filepath.Dir(dir)); err != nil {
		return err
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	file, err := os.CreateTemp(dir, ".restore-state-")
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err = file.Sync(); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err = file.Close(); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err = os.Rename(file.Name(), markerPath(dir)); err != nil {
		return ErrRestoreRecoveryRequired
	}
	return syncArchiveDir(dir)
}
func clearRestoreMarker(dir string) error {
	if err := os.Remove(markerPath(dir)); err != nil {
		return ErrRestoreRecoveryRequired
	}
	return syncArchiveDir(dir)
}
func syncArchiveDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	defer file.Close()
	if err = file.Sync(); err != nil {
		return ErrRestoreRecoveryRequired
	}
	return nil
}
func restoreStage(path, id string) string { return path + ".restore-" + id }

// preparePair writes both immutable recovery inputs on the live filesystem.
// Safety copies are independent of the archive volume and retained after success.
func preparePair(ctx context.Context, cfg *config.Config, path, id, activityID string) (restoreMarker, error) {
	marker := restoreMarker{Version: 1, BackupID: id, RestoreID: uuid.New().String(), ActivityID: activityID, Phase: "applying", DatabasePath: path}
	stage := restoreStage(path, marker.RestoreID)
	if err := os.Mkdir(stage, 0700); err != nil {
		return marker, ErrRestoreRecoveryRequired
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	name, _ := backuptypes.ArchiveNameForID(id)
	info, err := extractArchive(ctx, filepath.Join(cfg.BackupDir, name), stage, mustNewestMigration())
	if err != nil {
		return marker, err
	}
	marker.ArchiveSize, marker.MigrationVersion = info.size, info.migration
	for i, live := range []string{path, sqlitestore.FrancisPath(path)} {
		image := []string{hubImageName, francisImageName}[i]
		item, err := describeImage(ctx, filepath.Join(stage, image), image)
		if err != nil {
			return marker, err
		}
		marker.Images = append(marker.Images, item)
		// A safety copy is complete only after every committed WAL frame is in
		// the main file. Fail closed if checkpointing left any WAL behind.
		if wal, err := os.Lstat(live + "-wal"); err == nil {
			if !wal.Mode().IsRegular() || wal.Size() != 0 {
				return marker, ErrRestoreRecoveryRequired
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return marker, ErrRestoreRecoveryRequired
		}
		// Both pools are closed; ordinary file access cannot break a live SQLite lock.
		if _, err = os.Stat(live); err == nil {
			if err = copyDurable(ctx, live, filepath.Join(stage, image+".safety")); err != nil {
				return marker, ErrRestoreRecoveryRequired
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return marker, ErrRestoreRecoveryRequired
		}
	}
	if syncArchiveDir(stage) != nil || syncArchiveDir(filepath.Dir(path)) != nil {
		return marker, ErrRestoreRecoveryRequired
	}
	if err := writeRestoreMarker(restoreStateDir(path), marker); err != nil {
		return marker, err
	}
	keep = true
	return marker, nil
}
func copyDurable(ctx context.Context, source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, &contextReader{ctx: ctx, reader: in})
	syncErr := out.Sync()
	closeErr := out.Close()
	return errors.Join(copyErr, syncErr, closeErr)
}

// Roll forward from persistent inputs after any interruption. Reinstall both
// images even if one rename already completed; never infer progress from existence.
func installPair(ctx context.Context, dir string, marker restoreMarker) error {
	stage := restoreStage(marker.DatabasePath, marker.RestoreID)
	for _, item := range marker.Images {
		actual, err := describeImage(ctx, filepath.Join(stage, item.Name), item.Name)
		if err != nil || actual != item {
			return ErrRestoreRecoveryRequired
		}
	}
	if version, err := validateImages(ctx, stage, mustNewestMigration()); err != nil || version != marker.MigrationVersion {
		return ErrRestoreRecoveryRequired
	}
	for i, live := range []string{marker.DatabasePath, sqlitestore.FrancisPath(marker.DatabasePath)} {
		if err := sqlitestore.ValidateFile(live); err != nil {
			return ErrRestoreRecoveryRequired
		}
		// WAL and SHM belong to the replaced files; every connection is already closed.
		for _, suffix := range []string{"-wal", "-shm", ".install"} {
			if err := os.Remove(live + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return ErrRestoreRecoveryRequired
			}
		}
		if err := copyDurable(ctx, filepath.Join(stage, marker.Images[i].Name), live+".install"); err != nil {
			return ErrRestoreRecoveryRequired
		}
		if err := os.Rename(live+".install", live); err != nil {
			return ErrRestoreRecoveryRequired
		}
		if err := syncArchiveDir(filepath.Dir(live)); err != nil {
			return err
		}
	}
	marker.Phase = "verifying"
	return writeRestoreMarker(dir, marker)
}

// RecoverPendingRestore runs under the lifetime lock before migrations, HTTP or
// actors. A malformed marker or damaged staged image keeps startup unready.
func RecoverPendingRestore(ctx context.Context, cfg *config.Config) error {
	path, err := sqlitestore.Path(cfg.DatabaseURL)
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	dir := restoreStateDir(path)
	if CheckRestoreMarker(dir) == nil {
		return nil
	}
	marker, err := readRestoreMarker(dir)
	if err != nil {
		return err
	}
	lock, lockedPath, err := database.AcquireRestoreLock(ctx, cfg.DatabaseURL)
	if err != nil {
		return restoreLockError(err)
	}
	defer lock.Stop()
	if marker.DatabasePath != lockedPath {
		return ErrRestoreRecoveryRequired
	}
	if err = installPair(ctx, dir, marker); err != nil {
		return err
	}
	db, err := database.OpenLockedForRestore(ctx, path, lock)
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	defer db.SuspendOrdinaryConnections()
	if err = finalizeRestoredDatabase(ctx, db, cfg); err != nil {
		return err
	}
	if err = recordRecoveredArchive(ctx, db, marker); err != nil {
		return err
	}
	return clearRestoreMarker(dir)
}
func restoreLockError(err error) error {
	if errors.Is(err, database.ErrHubAlreadyRunning) {
		return database.ErrHubAlreadyRunning
	}
	return ErrRestoreRecoveryRequired
}
