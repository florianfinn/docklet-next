// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original bounded SQLite archive storage for issue #32.

package backup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

const archiveDeadline = time.Hour
const archiveSpaceMargin int64 = 64 << 20

var errArchiveLimit = errors.New("backup.size_limit")
var errArchiveTimeout = errors.New("backup.timeout")

// limitedArchiveWriter bounds bytes while the archive streams; it never buffers the archive.
type limitedArchiveWriter struct {
	file    io.Writer
	written int64
	limit   int64
}

func (w *limitedArchiveWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.limit-w.written {
		return 0, errArchiveLimit
	}
	n, err := w.file.Write(data)
	w.written += int64(n)
	return n, err
}

func ensureArchiveDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return ErrSystemBackupUnavailable
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ErrSystemBackupUnavailable
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrSystemBackupUnavailable
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return ErrSystemBackupUnavailable
	}
	return nil
}

func storageAvailable(dir string) error {
	if err := ensureArchiveDir(dir); err != nil {
		return err
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return ErrSystemBackupUnavailable
	}
	if int64(stat.Bavail)*int64(stat.Bsize) < backuptypes.MaxSystemArchiveBytes+archiveSpaceMargin {
		return ErrSystemBackupUnavailable
	}
	return nil
}

func requireFreeSpace(dir string, bytes int64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil || int64(stat.Bavail)*int64(stat.Bsize) < bytes+archiveSpaceMargin {
		return ErrSystemBackupUnavailable
	}
	return nil
}

func storageStatus(dir string) backuptypes.SystemBackupStorage {
	status := backuptypes.SystemBackupStorage{MaxArchiveBytes: backuptypes.MaxSystemArchiveBytes}
	if err := storageAvailable(dir); err != nil {
		status.ReasonCode = backuptypes.SystemBackupStorageMissing
		return status
	}
	var stat syscall.Statfs_t
	if syscall.Statfs(dir, &stat) != nil {
		status.ReasonCode = backuptypes.SystemBackupStorageMissing
		return status
	}
	status.Available = true
	status.FreeBytes = int64(stat.Bavail) * int64(stat.Bsize)
	entries, err := os.ReadDir(dir)
	if err != nil {
		status.Available = false
		status.ReasonCode = backuptypes.SystemBackupStorageMissing
		return status
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".tar") {
			continue
		}
		if info, err := entry.Info(); err == nil {
			status.UsedBytes += info.Size()
		}
	}
	return status
}

func archiveContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errArchiveTimeout
	}
	if ctx.Err() != nil {
		return errors.New("backup.interrupted")
	}
	return nil
}

func (s *Service) backupDir() string {
	if s.config != nil && s.config.BackupDir != "" {
		return s.config.BackupDir
	}
	return "/var/lib/docklet/backups"
}
func (s *Service) SupportedDatabaseProvider() bool {
	return s.databaseProviderError() == nil
}

func (s *Service) databaseProviderError() error {
	if s == nil || s.config == nil || s.config.DatabaseURL == "" || s.db == nil || s.db.Path() == "" {
		return ErrSystemBackupUnavailable
	}
	if err := s.archiveDirectoryError(); err != nil {
		return err
	}
	return storageAvailable(s.backupDir())
}

// Resolve both sides before comparing directory boundaries, including aliases
// through symlinked ancestors. Sibling names with a shared prefix are separate.
func validateArchiveSeparation(dir, databasePath string) error {
	archive, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ErrSystemBackupUnavailable
	}
	database, err := filepath.EvalSymlinks(databasePath)
	if err != nil {
		return ErrSystemBackupUnavailable
	}
	data := filepath.Dir(database)
	for _, paths := range [][2]string{{data, archive}, {archive, data}} {
		rel, err := filepath.Rel(paths[0], paths[1])
		if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return ErrSystemBackupUnavailable
		}
	}
	return nil
}

func (s *Service) archiveDirectoryError() error {
	if err := ensureArchiveDir(s.backupDir()); err != nil {
		return err
	}
	if s.db != nil {
		return validateArchiveSeparation(s.backupDir(), s.db.Path())
	}
	return nil
}

func (s *Service) Storage() backuptypes.SystemBackupStorage {
	if err := s.archiveDirectoryError(); err != nil {
		return backuptypes.SystemBackupStorage{MaxArchiveBytes: backuptypes.MaxSystemArchiveBytes, ReasonCode: backuptypes.SystemBackupStorageMissing}
	}
	return storageStatus(s.backupDir())
}
