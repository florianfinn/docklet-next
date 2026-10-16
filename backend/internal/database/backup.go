// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package database

import (
	"context"
	"database/sql"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
)

func LatestMigrationVersion() (int64, error) {
	return getHighestEmbeddedMigrationVersionInternal(dbProviderSQLite)
}
func (db *DB) Migrate(ctx context.Context) error {
	pool, err := db.SQLDB()
	if err != nil {
		return err
	}
	return migrateDatabaseInternal(ctx, pool, dbProviderSQLite, MigrationOptions{})
}

// OpenHubForRestore acquires the lifetime lock before opening either database.
// The caller can replace files without releasing that stable lock inode.
func OpenHubForRestore(ctx context.Context, databaseURL string) (*DB, error) {
	path, err := sqlitestore.Path(databaseURL)
	if err != nil {
		return nil, err
	}
	lock, err := acquireHubLock(ctx, path)
	if err != nil {
		return nil, err
	}
	db, err := connectDatabaseInternal(ctx, databaseURL)
	if err != nil {
		_ = lock.Stop()
		return nil, err
	}
	db.hubLock = lock
	return db, nil
}
func (db *DB) Path() string { return db.path }
func (db *DB) SuspendOrdinaryConnections() error {
	pool, err := db.SQLDB()
	if err != nil {
		return err
	}
	return pool.Close()
}

// Preserve the GORM root pointer used by domain services; all old handles have
// been drained and closed before replacement under the maintenance barrier.
func (db *DB) ResumeOrdinaryConnections() error {
	next, err := connectDatabaseInternal(context.Background(), sqlitestore.URL(db.path))
	if err != nil {
		return err
	}
	*db.DB = *next.DB
	return nil
}
func ValidateSnapshot(ctx context.Context, pool *sql.DB, newest int64) error {
	var count int
	if err := pool.QueryRowContext(ctx, "SELECT count(*) FROM docklet_database_metadata").Scan(&count); err != nil || count != 1 {
		return errors.New("backup.invalid_archive")
	}
	return validateIdentity(ctx, pool, newest)
}

func AcquireRestoreLock(ctx context.Context, databaseURL string) (*HubLock, string, error) {
	path, err := sqlitestore.Path(databaseURL)
	if err != nil {
		return nil, "", err
	}
	lock, err := acquireHubLock(ctx, path)
	return lock, path, err
}
func OpenLockedForRestore(ctx context.Context, path string, lock *HubLock) (*DB, error) {
	db, err := connectDatabaseInternal(ctx, sqlitestore.URL(path))
	if err != nil {
		return nil, err
	}
	db.hubLock = lock
	return db, nil
}
