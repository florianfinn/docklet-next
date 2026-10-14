// Source: florianfinn/docklet-next/backend/internal/database/database.go
// Revision: ab701984e86075694baabdc5efe14bed287768fc
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: New dedicated PostgreSQL session lock for single-hub startup.

package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
)

const (
	hubLockNamespace = 0x444f434b
	hubLockID        = 0x48554231
)

var ErrHubAlreadyRunning = errors.New("hub.already_running")

// HubLock owns a physical PostgreSQL connection for one hub lifetime.
type HubLock struct{ conn *sql.Conn }

func acquireHubLock(ctx context.Context, pool *sql.DB) (*HubLock, error) {
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire hub lock connection: %w", err)
	}
	var granted bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1, $2)", hubLockNamespace, hubLockID).Scan(&granted); err != nil {
		_ = discardHubLockConn(conn)
		return nil, fmt.Errorf("acquire hub session lock: %w", err)
	}
	if !granted {
		_ = conn.Close()
		return nil, ErrHubAlreadyRunning
	}
	return &HubLock{conn: conn}, nil
}

func discardHubLockConn(conn *sql.Conn) error {
	// Discard the physical session so its advisory lock cannot enter the pool.
	err := conn.Raw(func(any) error { return driver.ErrBadConn })
	if errors.Is(err, driver.ErrBadConn) {
		err = nil
	}
	closeErr := conn.Close()
	if errors.Is(closeErr, sql.ErrConnDone) {
		closeErr = nil
	}
	return errors.Join(err, closeErr)
}

func (l *HubLock) Stop() error {
	if l == nil {
		return nil
	}
	return discardHubLockConn(l.conn)
}

func (db *DB) HubLock() *HubLock {
	if db == nil {
		return nil
	}
	return db.hubLock
}

// AcquireHubLock attaches the lifetime lock to an existing migrated database.
func (db *DB) AcquireHubLock(ctx context.Context) error {
	if db == nil || db.hubLock != nil {
		return errors.New("hub lock already attached or database unavailable")
	}
	pool, err := db.SQLDB()
	if err != nil {
		return err
	}
	lock, err := acquireHubLock(ctx, pool)
	if err != nil {
		return err
	}
	db.hubLock = lock
	return nil
}
