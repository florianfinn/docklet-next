// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original bounded PostgreSQL archive writer for issue #32.

package backup

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

const archiveDeadline = time.Hour
const archiveSpaceMargin int64 = 64 << 20

var errArchiveLimit = errors.New("backup.size_limit")
var errArchiveTimeout = errors.New("backup.timeout")

// limitedArchiveWriter bounds bytes while pg_dump streams; it never buffers the archive.
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

func archiveToolsAvailable() bool {
	for _, name := range []string{"pg_dump", "pg_restore"} {
		path, err := exec.LookPath(name)
		if err != nil {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		output, err := exec.CommandContext(ctx, path, "--version").Output()
		cancel()
		if err != nil || !strings.Contains(string(output), "(PostgreSQL) 18.") {
			return false
		}
	}
	return true
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

func storageStatus(dir string) backuptypes.SystemBackupStorage {
	status := backuptypes.SystemBackupStorage{MaxArchiveBytes: backuptypes.MaxSystemArchiveBytes}
	if !archiveToolsAvailable() {
		status.ReasonCode = backuptypes.SystemBackupToolsUnavailable
		return status
	}
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
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".dump") {
			continue
		}
		if info, err := entry.Info(); err == nil {
			status.UsedBytes += info.Size()
		}
	}
	return status
}

func postgresEnv(dsn string) ([]string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" || parsed.Path == "" {
		return nil, ErrSystemBackupUnavailable
	}
	env := make([]string, 0, len(os.Environ())+8)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "PG") {
			env = append(env, item)
		}
	}
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return nil, ErrSystemBackupUnavailable
	}
	user := ""
	password := ""
	if parsed.User != nil {
		user = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	env = append(env, "PGHOST="+parsed.Hostname(), "PGPORT="+port, "PGUSER="+user,
		"PGPASSWORD="+password, "PGDATABASE="+strings.TrimPrefix(parsed.Path, "/"))
	for _, key := range []string{"sslmode", "sslrootcert", "sslcert", "sslkey", "connect_timeout"} {
		value := parsed.Query().Get(key)
		if value == "" {
			continue
		}
		name := map[string]string{"sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY", "connect_timeout": "PGCONNECT_TIMEOUT"}[key]
		env = append(env, name+"="+value)
	}
	return env, nil
}

func createArchive(ctx context.Context, dir, dsn, id string) (string, int64, error) {
	return createArchiveWithLimits(ctx, dir, dsn, id, backuptypes.MaxSystemArchiveBytes, archiveDeadline)
}

func createArchiveWithLimits(ctx context.Context, dir, dsn, id string, maxBytes int64, timeout time.Duration) (string, int64, error) {
	if err := archiveContextError(ctx); err != nil {
		return "", 0, err
	}
	name, err := backuptypes.ArchiveNameForID(id)
	if err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	if err := storageAvailable(dir); err != nil {
		return "", 0, err
	}
	env, err := postgresEnv(dsn)
	if err != nil {
		return "", 0, err
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	file, err := os.CreateTemp(dir, ".partial-")
	if err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	temp := file.Name()
	defer os.Remove(temp)
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	cmd := exec.CommandContext(deadlineCtx, "pg_dump", "--format=custom", "--no-owner", "--no-acl")
	cmd.Env, cmd.Stderr = env, io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	if err := cmd.Start(); err != nil {
		if contextErr := archiveContextError(deadlineCtx); contextErr != nil {
			return "", 0, contextErr
		}
		return "", 0, ErrSystemBackupUnavailable
	}
	writer := &limitedArchiveWriter{file: file, limit: maxBytes}
	_, copyErr := io.Copy(writer, stdout)
	if copyErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if contextErr := archiveContextError(deadlineCtx); contextErr != nil {
		return "", 0, contextErr
	}
	if copyErr != nil {
		if errors.Is(copyErr, errArchiveLimit) {
			return "", 0, errArchiveLimit
		}
		return "", 0, errors.New("backup.failed")
	}
	if waitErr != nil {
		return "", 0, errors.New("backup.failed")
	}
	if writer.written < 5 {
		return "", 0, errors.New("backup.invalid_archive")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", 0, errors.New("backup.failed")
	}
	header := make([]byte, 5)
	if _, err := io.ReadFull(file, header); err != nil || string(header) != "PGDMP" {
		return "", 0, errors.New("backup.invalid_archive")
	}
	verify := exec.CommandContext(deadlineCtx, "pg_restore", "--list", temp)
	verify.Env, verify.Stdout, verify.Stderr = env, io.Discard, io.Discard
	if err := verify.Run(); err != nil {
		return "", 0, errors.New("backup.invalid_archive")
	}
	if err := file.Sync(); err != nil {
		return "", 0, errors.New("backup.failed")
	}
	if err := file.Close(); err != nil {
		return "", 0, errors.New("backup.failed")
	}
	if err := os.Rename(temp, filepath.Join(dir, name)); err != nil {
		return "", 0, errors.New("backup.failed")
	}
	parent, err := os.Open(dir)
	if err != nil {
		return "", 0, errors.New("backup.failed")
	}
	err = parent.Sync()
	_ = parent.Close()
	if err != nil {
		_ = os.Remove(filepath.Join(dir, name))
		return "", 0, errors.New("backup.failed")
	}
	return name, writer.written, nil
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
	if !archiveToolsAvailable() {
		return ErrSystemBackupToolsUnavailable
	}
	if s == nil || s.config == nil || s.config.DatabaseURL == "" {
		return ErrSystemBackupUnavailable
	}
	return storageAvailable(s.backupDir())
}
