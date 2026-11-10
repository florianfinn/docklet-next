// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original SQLite restart proof for the self-update owner.

package upgrade

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestHubRunSurvivesSQLiteReopenAndDelayedResult(t *testing.T) {
	path := sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
	db, err := database.Initialize(t.Context(), path, database.MigrationOptions{})
	require.NoError(t, err)
	service := NewService(db, nil, nil, &config.Config{})
	op := core.Operation{OpID: "hub-restart", Role: "hub", EnvironmentID: "0", TargetRef: "example.com/docklet-hub:1.0.1@sha256:" + testDigest, TargetVersion: "1.0.1", Deadline: time.Now().Add(core.OperationTimeout)}
	op, created, err := service.reserveRun(t.Context(), service.store, op, "operator", "operator")
	require.NoError(t, err)
	require.True(t, created)
	require.NotEmpty(t, op.RunID)
	_, err = service.store.Change(t.Context(), op.OpID, func(_ *gorm.DB, current *core.Operation) error { current.Phase = "helper-started"; return nil })
	require.NoError(t, err)
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), path, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	service = NewService(db, nil, nil, &config.Config{})
	service.runningVersion = func() string { return "1.0.1" }
	service.data = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(service.data, "self-update"), 0750))
	ready := make(chan error, 1)
	go func() { ready <- service.Start(t.Context()) }()
	select {
	case err := <-ready:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("startup waited for the helper result")
	}
	service.Stop()
	// Readiness is independent: startup, repair and a read complete before any result exists.
	repaired, err := activity.NewActivityService(db, nil).RepairStartup(t.Context())
	require.NoError(t, err)
	require.Zero(t, repaired)
	var run activity.Activity
	require.NoError(t, db.First(&run, "id=?", op.RunID).Error)
	require.Equal(t, "running", string(run.Status))
	data := service.data
	require.NoError(t, os.MkdirAll(filepath.Join(data, "self-update"), 0750))
	observer := core.Observer{Store: service.store, Data: data, Version: service.runningVersion, Running: func(context.Context, core.Operation) (bool, error) { return true, nil }, Settle: service.settleRun}
	done, err := observer.Advance(t.Context())
	require.NoError(t, err)
	require.False(t, done, "a ready target-version hub is not a health result")
	require.NoError(t, core.WriteJSON(core.ResultPath(data, op.OpID), core.Result{OpID: op.OpID, TargetRef: op.TargetRef, Phase: "healthy"}))
	done, err = observer.Advance(t.Context())
	require.NoError(t, err)
	require.True(t, done)
	require.NoError(t, db.First(&run, "id=?", op.RunID).Error)
	require.Equal(t, "success", string(run.Status))
	ended := *run.EndedAt
	observer.Settle = func(*gorm.DB, core.Operation) error { t.Fatal("terminal callback repeated"); return nil }
	done, err = observer.Advance(t.Context())
	require.NoError(t, err)
	require.True(t, done)
	require.NoError(t, db.First(&run, "id=?", op.RunID).Error)
	require.Equal(t, ended, *run.EndedAt)
}
