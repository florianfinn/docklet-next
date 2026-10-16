// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Independent-connection proof that writer transactions reserve before decision reads.

package sqlitestore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriterBeginsImmediateBeforeDecisionRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "immediate.db")
	writer, err := Open(t.Context(), path, true, false)
	require.NoError(t, err)
	defer writer.Close()
	_, err = writer.Exec("CREATE TABLE sample(value INTEGER); INSERT INTO sample VALUES(1)")
	require.NoError(t, err)
	raw, err := sql.Open("sqlite", URL(path))
	require.NoError(t, err)
	defer raw.Close()
	conn, err := raw.Conn(t.Context())
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(t.Context(), "BEGIN IMMEDIATE")
	require.NoError(t, err)
	defer conn.ExecContext(t.Context(), "ROLLBACK")
	_, err = conn.ExecContext(t.Context(), "UPDATE sample SET value=2")
	require.NoError(t, err)
	type outcome struct {
		tx  *sql.Tx
		err error
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan outcome, 1)
	go func() {
		tx, err := writer.BeginTx(ctx, nil)
		result <- outcome{tx, err}
	}()
	select {
	case got := <-result:
		if got.tx != nil {
			_ = got.tx.Rollback()
		}
		t.Fatalf("writer began before the independent RESERVED lock was released: %v", got.err)
	case <-time.After(100 * time.Millisecond):
	}
	_, err = conn.ExecContext(t.Context(), "COMMIT")
	require.NoError(t, err)
	select {
	case got := <-result:
		require.NoError(t, got.err)
		defer got.tx.Rollback()
		var value int
		require.NoError(t, got.tx.QueryRowContext(t.Context(), "SELECT value FROM sample").Scan(&value))
		require.Equal(t, 2, value)
		require.NoError(t, got.tx.Commit())
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not begin after the independent transaction committed")
	}
}
