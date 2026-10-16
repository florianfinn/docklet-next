// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package backup

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

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

func validateArchive(ctx context.Context, path string, newest int64) (archiveInfo, error) {
	dir, err := os.MkdirTemp(filepath.Dir(path), ".validate-")
	if err != nil {
		return archiveInfo{}, ErrSystemBackupUnavailable
	}
	defer os.RemoveAll(dir)
	return extractArchive(ctx, path, dir, newest)
}

// Only three regular members in a fixed order are accepted. No archive path,
// link, mode or filename is passed to the filesystem as supplied by an upload.
func extractArchive(ctx context.Context, path, dir string, newest int64) (archiveInfo, error) {
	var info archiveInfo
	if err := archiveContextError(ctx); err != nil {
		return info, err
	}
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() < 1024 {
		return info, errInvalidArchive
	}
	if stat.Size() > backuptypes.MaxSystemArchiveBytes {
		return info, errArchiveLimit
	}
	file, err := os.Open(path)
	if err != nil {
		return info, errInvalidArchive
	}
	defer file.Close()
	if stat.Size()%512 != 0 {
		return info, errInvalidArchive
	}
	footer := make([]byte, 1024)
	if _, err = file.ReadAt(footer, stat.Size()-1024); err != nil {
		return info, errInvalidArchive
	}
	for _, b := range footer {
		if b != 0 {
			return info, errInvalidArchive
		}
	}
	tr := tar.NewReader(&contextReader{ctx: ctx, reader: file})
	h, err := tr.Next()
	if err != nil || h.Typeflag != tar.TypeReg || h.Name != "manifest.json" || h.Size > 16384 || h.Size < 1 {
		return info, errInvalidArchive
	}
	var manifest archiveManifest
	decoder := json.NewDecoder(io.LimitReader(tr, 16385))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(new(any)) != io.EOF {
		return info, errInvalidArchive
	}
	if manifest.Format != "docklet-sqlite" || manifest.Version != archiveVersion {
		return info, errArchiveMajor
	}
	if manifest.HubSchema < 1 || manifest.HubSchema > newest || manifest.FrancisSchema != francisSchemaVersion {
		return info, errMigration
	}
	if len(manifest.Images) != 2 {
		return info, errInvalidArchive
	}
	var total int64
	for i, name := range []string{hubImageName, francisImageName} {
		item := manifest.Images[i]
		if item.Name != name || item.Size < 512 || item.Size > backuptypes.MaxSystemArchiveBytes-total || len(item.SHA256) != 64 {
			return info, errInvalidArchive
		}
		total += item.Size
		h, err = tr.Next()
		if err != nil || h.Typeflag != tar.TypeReg || h.Name != name || h.Size != item.Size {
			return info, errInvalidArchive
		}
		output, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return info, errInvalidArchive
		}
		_, err = io.Copy(output, tr)
		syncErr := output.Sync()
		closeErr := output.Close()
		if errors.Join(err, syncErr, closeErr) != nil {
			return info, archiveValidationError(ctx)
		}
		actual, err := describeImage(ctx, filepath.Join(dir, name), name)
		if err != nil || actual != item {
			return info, errInvalidArchive
		}
	}
	if _, err = tr.Next(); err != io.EOF {
		return info, errInvalidArchive
	}
	// Tar permits zero padding only after its end marker.
	buffer := make([]byte, 4096)
	for {
		n, e := file.Read(buffer)
		for _, b := range buffer[:n] {
			if b != 0 {
				return info, errInvalidArchive
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return info, errInvalidArchive
		}
	}
	version, err := validateImages(ctx, dir, newest)
	if err != nil {
		return info, err
	}
	hub, openErr := openImage(filepath.Join(dir, hubImageName))
	if openErr != nil {
		return info, errInvalidArchive
	}
	snapshotID, idErr := imageSnapshotID(ctx, hub, hubImageName)
	_ = hub.Close()
	if idErr != nil || snapshotID != manifest.SnapshotID {
		return info, errInvalidArchive
	}
	if version != manifest.HubSchema {
		return info, errMigration
	}
	return archiveInfo{size: stat.Size(), migration: version}, nil
}
func archiveValidationError(ctx context.Context) error {
	if err := archiveContextError(ctx); err != nil {
		return err
	}
	return errInvalidArchive
}
func newestMigration() (int64, error) { return database.LatestMigrationVersion() }
