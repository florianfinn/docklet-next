// Source: getarcaneapp/arcane/backend/pkg/projects/fs_backup.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package projects

import (
	"context"
	"errors"
	"fmt"
	"go.getarcane.app/acfs"
	acfstypes "go.getarcane.app/acfs/types"
	"os"
	"path/filepath"
	"strings"
)

func RestoreProjectUpdateBackup(ctx context.Context, projectDir string, backup *ProjectUpdateBackup) error {
	projRoot, err := os.OpenRoot(projectDir)
	if err != nil {
		return fmt.Errorf("open project directory: %w", err)
	}
	defer func() { _ = projRoot.Close() }()

	// 1. Undo directory renames, then 2. remove debris the failed update
	// created at paths that did not exist.
	if undoRenamedDirsAndDebrisErr := undoRenamedDirsAndDebrisInternal(projRoot, backup); undoRenamedDirsAndDebrisErr != nil {
		return undoRenamedDirsAndDebrisErr
	}

	var failures []error
	// 3. Mirror backed-up directories back in place, clearing a file the
	// failed update may have put where the directory was.
	for _, rel := range backup.DirEntries {
		if live, lstatErr := projRoot.Lstat(rel); lstatErr == nil && !live.IsDir() {
			if removeErr := projRoot.Remove(rel); removeErr != nil {
				failures = append(failures, fmt.Errorf("remove conflicting file %s: %w", rel, removeErr))
				continue
			}
		}
		if mkdirAllErr := projRoot.MkdirAll(rel, 0o755); mkdirAllErr != nil {
			failures = append(failures, FilesystemError(ctx, "recreate project directory", filepath.Join(projectDir, rel), mkdirAllErr))
			continue
		}
		preserve := skippedUnderInternal(backup.Skipped, rel)
		src := filepath.Join(backup.BackupDir, filepath.FromSlash(rel))
		dest := filepath.Join(projectDir, filepath.FromSlash(rel))
		if mirrorDirErr := acfs.MirrorDir(ctx, src, dest, acfstypes.MirrorOptions{Preserve: preserve}); mirrorDirErr != nil {
			failures = append(failures, fmt.Errorf("restore project directory %s: %w", rel, mirrorDirErr))
		}
	}

	backupRoot, err := os.OpenRoot(backup.BackupDir)
	if err != nil {
		return errors.Join(append(failures, fmt.Errorf("open backup directory: %w", err))...)
	}
	defer func() { _ = backupRoot.Close() }()

	// 4. Restore individual files in place.
	for _, rel := range backup.FileEntries {
		if restoreBackupFileErr := restoreBackupFileInternal(ctx, backupRoot, projRoot, backup, rel); restoreBackupFileErr != nil {
			failures = append(failures, restoreBackupFileErr)
		}
	}

	// 5. Depth-1 mini-mirror of top-level regular files: prune live top-level
	// regular files absent from the backup (and not skipped as unreadable),
	// then copy every top-level backup file back. Top-level directories and
	// symlinks are never touched.
	if backup.TopLevelFiles {
		if restoreTopLevelFilesErr := restoreTopLevelFilesInternal(ctx, backupRoot, projRoot, backup); restoreTopLevelFilesErr != nil {
			failures = append(failures, restoreTopLevelFilesErr)
		}
	}

	return errors.Join(failures...)
}

func restoreTopLevelFilesInternal(ctx context.Context, backupRoot, projRoot *os.Root, backup *ProjectUpdateBackup) error {
	var failures []error
	backupEntries, err := os.ReadDir(backupRoot.Name())
	if err != nil {
		return fmt.Errorf("read backup directory: %w", err)
	}
	inBackup := make(map[string]bool, len(backupEntries))
	for _, entry := range backupEntries {
		if entry.Type().IsRegular() {
			inBackup[entry.Name()] = true
		}
	}
	skippedTopLevel := make(map[string]bool)
	for _, s := range backup.Skipped {
		if !strings.Contains(s, "/") {
			skippedTopLevel[s] = true
		}
	}

	liveEntries, err := os.ReadDir(projRoot.Name())
	if err != nil {
		return fmt.Errorf("read project directory: %w", err)
	}
	for _, entry := range liveEntries {
		name := entry.Name()
		if !entry.Type().IsRegular() || inBackup[name] || skippedTopLevel[name] {
			continue
		}
		if removeErr := projRoot.Remove(name); removeErr != nil {
			failures = append(failures, fmt.Errorf("prune created file %s: %w", name, removeErr))
		}
	}

	// backupEntries is sorted by os.ReadDir, so restoration order is
	// deterministic and mirrors the backup order.
	for _, entry := range backupEntries {
		if !entry.Type().IsRegular() {
			continue
		}
		if restoreBackupFileErr := restoreBackupFileInternal(ctx, backupRoot, projRoot, backup, entry.Name()); restoreBackupFileErr != nil {
			failures = append(failures, restoreBackupFileErr)
		}
	}
	return errors.Join(failures...)
}
