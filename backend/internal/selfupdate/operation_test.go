// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openStore(t *testing.T, path string) *Store {
	t.Helper()
	db, err := gorm.Open(testsqlite.Dialector(t, path), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	return NewStore(&database.DB{DB: db})
}
func closeStore(t *testing.T, s *Store) {
	t.Helper()
	db, err := s.DB.DB.DB()
	require.NoError(t, err)
	require.NoError(t, db.Close())
}
func operationFixture() Operation {
	return Operation{OpID: "operation-1", Role: "hub", RunID: "run-1", EnvironmentID: "0", OldContainerID: "old-container", TargetRef: "example.com/hub:1.0.1@sha256:" + digestHex, TargetVersion: "1.0.1", FromVersion: "1.0.0", Deadline: time.Now().Add(OperationTimeout)}
}
func TestDurableReopenWaitsForCorrelatedHealthResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := openStore(t, path)
	op := operationFixture()
	_, created, err := store.Reserve(t.Context(), op, nil)
	require.NoError(t, err)
	require.True(t, created)
	_, err = store.Change(t.Context(), op.OpID, func(_ *gorm.DB, op *Operation) error { op.Phase = "helper-started"; return nil })
	require.NoError(t, err)
	closeStore(t, store)
	store = openStore(t, path)
	defer closeStore(t, store)
	data := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(data, "self-update"), 0750))
	settlements := 0
	observer := Observer{Store: store, Data: data, Version: func() string { return "1.0.1" }, Running: func(_ context.Context, _ Operation) (bool, error) { return true, nil }, Settle: func(_ *gorm.DB, op Operation) error {
		settlements++
		require.Equal(t, "healthy", op.Outcome)
		return nil
	}}
	done, err := observer.Advance(t.Context())
	require.NoError(t, err)
	require.False(t, done, "new version and readiness are not helper health evidence")
	require.Zero(t, settlements)
	require.NoError(t, WriteJSON(ResultPath(data, op.OpID), Result{OpID: op.OpID, TargetRef: op.TargetRef, Phase: "healthy"}))
	done, err = observer.Advance(t.Context())
	require.NoError(t, err)
	require.True(t, done)
	require.Equal(t, 1, settlements)
	done, err = observer.Advance(t.Context())
	require.NoError(t, err)
	require.True(t, done)
	require.Equal(t, 1, settlements)
}
func TestForeignResultAndDeadline(t *testing.T) {
	for _, name := range []string{"wrong op", "wrong target", "missing", "healthy old binary", "failed health", "pre effect", "helper grace", "grace exhausted", "interrupted"} {
		t.Run(name, func(t *testing.T) {
			store := openStore(t, filepath.Join(t.TempDir(), "state.db"))
			defer closeStore(t, store)
			op := operationFixture()
			data := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(data, "self-update"), 0750))
			result := Result{OpID: op.OpID, TargetRef: op.TargetRef, Phase: "healthy"}
			version := "1.0.1"
			now := time.Now()
			running := true
			wantDone := false
			outcome := ""
			switch name {
			case "wrong op":
				result.OpID = "foreign"
			case "wrong target":
				result.TargetRef = "example.com/foreign:1.0.1"
			case "healthy old binary":
				version = "1.0.0"
			case "failed health":
				result.Phase = "failed-health"
				wantDone = true
				outcome = result.Phase
			case "pre effect":
				result.Phase = "failed-pre-effect"
				wantDone = true
				outcome = result.Phase
			case "helper grace":
				op.Deadline = now.Add(-time.Second)
				result.Phase = "up-started"
			case "grace exhausted":
				op.Deadline = now.Add(-2 * OperationTimeout)
				result.Phase = "up-started"
				wantDone = true
				outcome = "unknown"
			case "interrupted":
				result.Phase = "env-written"
				running = false
				wantDone = true
				outcome = "unknown"
			}
			_, _, err := store.Reserve(t.Context(), op, nil)
			require.NoError(t, err)
			if name != "missing" {
				require.NoError(t, WriteJSON(ResultPath(data, op.OpID), result))
			}
			observer := Observer{Store: store, Data: data, Version: func() string { return version }, Running: func(context.Context, Operation) (bool, error) { return running, nil }, Now: func() time.Time { return now }}
			done, err := observer.Advance(t.Context())
			require.NoError(t, err)
			require.Equal(t, wantDone, done)
			got, err := store.Read(t.Context())
			require.NoError(t, err)
			require.Equal(t, outcome, got.Outcome)
		})
	}
}
func TestOperationIdempotentAndBusy(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer closeStore(t, s)
	op := operationFixture()
	_, created, err := s.Reserve(t.Context(), op, nil)
	require.NoError(t, err)
	require.True(t, created)
	_, created, err = s.Reserve(t.Context(), op, nil)
	require.NoError(t, err)
	require.False(t, created)
	next := op
	next.OpID = "second-operation"
	_, _, err = s.Reserve(t.Context(), next, nil)
	require.ErrorContains(t, err, "host.busy")
	next = op
	next.TargetRef = "example.com/foreign:1.0.1"
	_, _, err = s.Reserve(t.Context(), next, nil)
	require.ErrorContains(t, err, "update.invalid-target")
}
