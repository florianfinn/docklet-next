// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Temp-file persistence fixtures using the production connection policy.

package testsqlite

import (
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"gorm.io/gorm"
	"path/filepath"
	"sync"
	"testing"
)

var fixtures = struct {
	sync.Mutex
	paths map[string]string
}{paths: map[string]string{}}

// Dialector returns a single-connection file pool. Reusing a fixture within
// a test reopens that file; :memory: remains an isolated file per invocation.
func Dialector(t testing.TB, fixture string) gorm.Dialector {
	t.Helper()
	fixtures.Lock()
	key := t.Name() + "\x00" + fixture
	path, ok := fixtures.paths[key]
	if !ok || fixture == ":memory:" {
		path = filepath.Join(t.TempDir(), "test.db")
		if fixture != ":memory:" {
			fixtures.paths[key] = path
			t.Cleanup(func() { fixtures.Lock(); defer fixtures.Unlock(); delete(fixtures.paths, key) })
		}
	}
	fixtures.Unlock()
	pool, err := sqlitestore.Open(t.Context(), path, true, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	return sqlitestore.Dialector(pool)
}

// BusyError obtains a real modernc error for retry-loop fault injection.
func BusyError(t testing.TB) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "busy.db")
	first, err := sqlitestore.Open(t.Context(), path, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := sqlitestore.Open(t.Context(), path, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err = second.Exec("PRAGMA busy_timeout=0"); err != nil {
		t.Fatal(err)
	}
	tx, err := first.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = second.Exec("CREATE TABLE blocked(id INTEGER)")
	if err == nil {
		t.Fatal("competing writer did not produce busy")
	}
	return err
}
