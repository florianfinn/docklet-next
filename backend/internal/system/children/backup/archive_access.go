// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced Rustic system recovery with coordinated SQLite archives; retained the separate volume-backup coordinator; notification and Auto Heal contracts and delivery for #296; M16 bind mounts and backup completion.

package backup

import (
	"context"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"io"
	"os"
	"path/filepath"
)

func (s *Service) OpenArchive(ctx context.Context, id string) (io.ReadCloser, int64, string, error) {
	name, err := backuptypes.ArchiveNameForID(id)
	if err != nil {
		return nil, 0, "", ErrSystemBackupNotFound
	}
	run, err := s.store.Run(ctx, id)
	if err != nil || run == nil || run.Status != backuptypes.SystemBackupStatusSucceeded || run.ArchiveName != name {
		return nil, 0, "", ErrSystemBackupNotFound
	}
	return s.openArchiveFile(run, name)
}

// Internal copy uses the owned archive while the public run still reports running.
func (s *Service) openArchiveFile(run *backuptypes.SystemBackupRun, name string) (io.ReadCloser, int64, string, error) {
	file, err := os.Open(filepath.Join(s.backupDir(), name))
	if err != nil {
		return nil, 0, "", ErrSystemBackupNotFound
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != run.Size {
		_ = file.Close()
		return nil, 0, "", ErrSystemBackupNotFound
	}
	return file, info.Size(), name, nil
}
