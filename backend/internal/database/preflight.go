// Source: docklet-next/issues/361
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original read-only startup classification before writable SQLite policy.

package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
)

var errPrerelease = errors.New("database.prerelease: a pre-release database cannot be upgraded; start with a new data directory")

// The lifetime lock is already held. Do not apply writable connection policy,
// chmod the database or inspect the independent Francis sidecar on refusal.
func preflightDatabase(ctx context.Context, path string, options MigrationOptions) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	target, err := LatestMigrationVersion(options)
	if err != nil {
		return err
	}
	inspectionPath, cleanup, err := preflightView(path)
	if err != nil {
		return err
	}
	defer cleanup()
	u, _ := url.Parse(sqlitestore.URL(inspectionPath))
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("cache", "private")
	if inspectionPath == path {
		q.Set("immutable", "1")
	}
	u.RawQuery = q.Encode()
	pool, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	pool.SetMaxOpenConns(1)
	err = validateIdentity(ctx, pool, target, options)
	err = errors.Join(err, pool.Close())
	if err != nil {
		return fmt.Errorf("database %q: %w", path, err)
	}
	return nil
}

// SQLite mode=ro may still create WAL/SHM files. With no WAL frames, immutable
// reads the main file alone. Otherwise inspect a private copy of the committed
// main/WAL pair; SQLite may build its own SHM there, never beside the input.
func preflightView(path string) (string, func(), error) {
	wal, err := os.Stat(path + "-wal")
	if errors.Is(err, os.ErrNotExist) || (err == nil && wal.Size() == 0) {
		return path, func() {}, nil
	}
	if err != nil {
		return "", nil, err
	}
	if err := sqlitestore.ValidateFile(path + "-wal"); err != nil {
		return "", nil, err
	}
	if !wal.Mode().IsRegular() {
		return "", nil, errors.New("database WAL must be a regular file")
	}
	dir := preflightViewDir(path)
	spaceErr := func(err error) error {
		return fmt.Errorf("database preflight needs temporary space in %s for a copy of %s and its WAL: %w", os.TempDir(), path, err)
	}
	// The caller holds this database's lifetime lock, so a leftover view of the
	// same path is stale (killed copy) and can be replaced.
	if err := os.RemoveAll(dir); err != nil {
		return "", nil, spaceErr(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return "", nil, spaceErr(err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	view := filepath.Join(dir, "inspect.db")
	for _, suffix := range []string{"", "-wal"} {
		if err := copyPreflightFile(path+suffix, view+suffix); err != nil {
			cleanup()
			return "", nil, spaceErr(err)
		}
	}
	return view, cleanup, nil
}

// preflightViewDir names one temporary view per database path.
func preflightViewDir(path string) string {
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(os.TempDir(), "docklet-preflight-"+hex.EncodeToString(sum[:8]))
}

func copyPreflightFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	return errors.Join(err, output.Close())
}

// ValidateExistingDatabase classifies a resolved file without writable SQLite
// policy. Restore callers must already hold the database's lifetime lock.
func ValidateExistingDatabase(ctx context.Context, path string, options ...MigrationOptions) error {
	migrationOptions := MigrationOptions{}
	if len(options) > 0 {
		migrationOptions = options[0]
	}
	return preflightDatabase(ctx, path, migrationOptions)
}
