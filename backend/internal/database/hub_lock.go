// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Descriptor-owned single-hub lock on a stable local inode.

package database

import (
	"context"
	"errors"
	"github.com/gofrs/flock"
	"os"
)

var ErrHubAlreadyRunning = errors.New("hub.already_running")

// HubLock holds flock (LockFileEx on Windows) until all database handles close.
type HubLock struct{ lock *flock.Flock }

func acquireHubLock(ctx context.Context, path string) (*HubLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path + ".lock")
	if err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("hub lock must be a regular file")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	lock := flock.New(path+".lock", flock.SetPermissions(0600))
	ok, err := lock.TryLock()
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	if !ok {
		_ = lock.Close()
		return nil, ErrHubAlreadyRunning
	}
	return &HubLock{lock: lock}, nil
}
func (l *HubLock) Stop() error {
	if l == nil {
		return nil
	}
	return l.lock.Close()
}
func (db *DB) HubLock() *HubLock {
	if db == nil {
		return nil
	}
	return db.hubLock
}

// AcquireHubLock is only for test fixtures already opened by the owner.
func (db *DB) AcquireHubLock(ctx context.Context) error {
	if db == nil || db.hubLock != nil || db.path == "" {
		return errors.New("hub lock already attached or database path unavailable")
	}
	lock, err := acquireHubLock(ctx, db.path)
	if err != nil {
		return err
	}
	db.hubLock = lock
	return nil
}
