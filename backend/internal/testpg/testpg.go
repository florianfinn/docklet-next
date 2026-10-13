// Source: florianfinn/docklet-next/backend/internal/testpg
// Revision: 3f11be42e99b779ad09971803f33f41c5fdc7255
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: New local PostgreSQL integration-test helper; no upstream source.

package testpg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var schemas = struct {
	sync.Mutex
	byTest map[string]string
	next   uint64
}{byTest: make(map[string]string)}

// Dialector isolates one integration test in a PostgreSQL schema. Reopening
// the same fixture key within a test uses the same schema.
func Dialector(t testing.TB, fixture string) gorm.Dialector {
	t.Helper()
	dsn := os.Getenv("DOCKLET_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("PostgreSQL integration requires DOCKLET_TEST_POSTGRES_DSN")
	}

	schemas.Lock()
	key := t.Name() + "\x00" + fixture
	if fixture == ":memory:" {
		schemas.next++
		key += fmt.Sprintf("\x00%d", schemas.next)
	}
	schema, ok := schemas.byTest[key]
	if !ok {
		var suffix [12]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			schemas.Unlock()
			t.Fatalf("generate test schema: %v", err)
		}
		schema = "test_" + hex.EncodeToString(suffix[:])
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		conn, err := pgx.Connect(ctx, dsn)
		if err == nil {
			_, err = conn.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schema))
			_ = conn.Close(ctx)
		}
		cancel()
		if err != nil {
			schemas.Unlock()
			t.Fatalf("create test schema: %v", err)
		}
		schemas.byTest[key] = schema
		t.Cleanup(func() {
			schemas.Lock()
			delete(schemas.byTest, key)
			schemas.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conn, err := pgx.Connect(ctx, dsn)
			if err != nil {
				t.Errorf("connect for schema cleanup: %v", err)
				return
			}
			defer conn.Close(ctx)
			if _, err := conn.Exec(ctx, fmt.Sprintf("DROP SCHEMA %s CASCADE", schema)); err != nil {
				t.Errorf("drop test schema: %v", err)
			}
		})
	}
	schemas.Unlock()

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse PostgreSQL integration DSN: %v", err)
	}
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = make(map[string]string)
	}
	cfg.RuntimeParams["search_path"] = schema
	sqlDB := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return postgres.New(postgres.Config{Conn: sqlDB})
}
