// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original PostgreSQL archive validation for issue #32.

package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

var (
	errInvalidArchive = errors.New("backup.invalid_archive")
	errArchiveMajor   = errors.New("backup.incompatible_server")
	errMigration      = errors.New("backup.incompatible_migration")
)

type archiveInfo struct {
	size      int64
	migration int64
}

// prefixWriter retains only a bounded prefix while pg_restore drains its TOC.
type prefixWriter struct {
	bytes.Buffer
	limit int
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := w.limit - w.Len(); remaining > 0 {
		_, _ = w.Buffer.Write(p[:min(n, remaining)])
	}
	return n, nil
}

func validateArchive(ctx context.Context, path string, newest int64) (archiveInfo, error) {
	var result archiveInfo
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() < 5 {
		return result, errInvalidArchive
	}
	if stat.Size() > backuptypes.MaxSystemArchiveBytes {
		return result, errArchiveLimit
	}
	file, err := os.Open(path)
	if err != nil {
		return result, errInvalidArchive
	}
	var signature [5]byte
	_, err = io.ReadFull(file, signature[:])
	_ = file.Close()
	if err != nil || string(signature[:]) != "PGDMP" {
		return result, errInvalidArchive
	}
	deadline, cancel := context.WithTimeout(ctx, archiveDeadline)
	defer cancel()
	var toc prefixWriter
	toc.limit = 16 << 10
	list := exec.CommandContext(deadline, "pg_restore", "--list", path)
	list.Stdout, list.Stderr = &toc, io.Discard
	if err := list.Run(); err != nil {
		return result, archiveValidationError(deadline)
	}
	major := 0
	for _, line := range strings.Split(toc.String(), "\n") {
		const label = ";     Dumped from database version: "
		if strings.HasPrefix(line, label) {
			part := strings.TrimPrefix(line, label)
			major, _ = strconv.Atoi(strings.SplitN(part, ".", 2)[0])
			break
		}
	}
	if major != 18 {
		return result, errArchiveMajor
	}
	var rows limitedArchiveWriter
	var data bytes.Buffer
	rows.file, rows.limit = &data, 256<<10
	extract := exec.CommandContext(deadline, "pg_restore", "--data-only", "--table=goose_db_version", "--file=-", path)
	extract.Stdout, extract.Stderr = &rows, io.Discard
	if err := extract.Run(); err != nil {
		return result, archiveValidationError(deadline)
	}
	migration, err := parseGooseArchive(data.String())
	if err != nil || migration < 1 || migration > newest {
		return result, errMigration
	}
	result.size, result.migration = stat.Size(), migration
	return result, nil
}

func archiveValidationError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errArchiveTimeout
	}
	return errInvalidArchive
}

func parseGooseArchive(sql string) (int64, error) {
	var version int64
	inCopy := false
	for _, line := range strings.Split(sql, "\n") {
		if strings.HasPrefix(line, "COPY public.goose_db_version (") && strings.HasSuffix(line, ") FROM stdin;") {
			inCopy = true
			continue
		}
		if !inCopy {
			continue
		}
		if line == `\.` {
			break
		}
		columns := strings.Split(line, "\t")
		if len(columns) < 3 {
			return 0, errMigration
		}
		parsed, err := strconv.ParseInt(columns[1], 10, 64)
		if err != nil {
			return 0, errMigration
		}
		if columns[2] == "t" && parsed > version {
			version = parsed
		}
	}
	if !inCopy || version < 1 {
		return 0, errMigration
	}
	return version, nil
}

func validateTargetVersion(ctx context.Context, db *database.DB, archiveVersion, newest int64, recovery bool) error {
	if db == nil || archiveVersion < 1 || archiveVersion > newest {
		return errMigration
	}
	var tables bool
	if err := db.WithContext(ctx).Raw("SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname='public')").Scan(&tables).Error; err != nil {
		return errMigration
	}
	if !tables {
		return nil
	}
	var current int64
	if err := db.WithContext(ctx).Raw("SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = true").Scan(&current).Error; err != nil {
		return errMigration
	}
	if current != archiveVersion && (!recovery || current < archiveVersion || current > newest) {
		return errMigration
	}
	return nil
}

func newestMigration() (int64, error) { return database.LatestMigrationVersion() }
