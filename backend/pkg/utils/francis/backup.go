// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package francis

import (
	"context"
	"database/sql"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
)

// ReserveWriter drains transactions and blocks every new writer on the pool.
// The caller also reserves the hub pool before taking either image.
func (r *Runtime) ReserveWriter(ctx context.Context) (*sql.Conn, error) {
	r.mu.Lock()
	pool := r.writer
	r.mu.Unlock()
	if pool == nil {
		return nil, errors.New("backup.storage_unavailable")
	}
	return pool.Conn(ctx)
}

// Restart reuses registered factories and the service pointer after offline
// replacement. Stop must have joined every host/provider handle first.
func (r *Runtime) Restart(ctx context.Context) error {
	r.mu.Lock()
	appCtx, onFailure := r.appCtx, r.onFailure
	r.started = false
	r.ready = make(chan struct{})
	r.done = make(chan struct{})
	r.runError = nil
	r.writer = nil
	r.mu.Unlock()
	return r.Start(ctx, appCtx, onFailure)
}

// InitializeStore creates an empty sidecar for a stopped hub or isolated test.
func InitializeStore(ctx context.Context, databaseURL string) error {
	storeURL, err := StoreURL(databaseURL)
	if err != nil {
		return err
	}
	provider, err := openProvider(ctx, storeURL)
	if err != nil {
		return err
	}
	return provider.Close()
}

func ClearRestoredOwnership(ctx context.Context, databaseURL string) error {
	if err := ClearRestoredHosts(ctx, databaseURL); err != nil {
		return err
	}
	path, err := sqlitestore.Path(databaseURL)
	if err != nil {
		return err
	}
	pool, err := sqlitestore.Open(ctx, sqlitestore.FrancisPath(path), false, false)
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err = pool.ExecContext(ctx, "DROP TABLE IF EXISTS docklet_backup_image"); err != nil {
		return err
	}
	_, err = pool.ExecContext(ctx, "UPDATE arcane_francis_alarms SET alarm_lease_id=NULL, alarm_lease_expiration_time=NULL")
	if err != nil {
		return err
	}
	_, err = pool.ExecContext(ctx, "UPDATE arcane_francis_cluster_config SET exclusive_owner=NULL,exclusive_expires_at=NULL")
	return err
}
