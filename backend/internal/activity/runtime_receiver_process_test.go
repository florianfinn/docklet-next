// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Agent-initialized separate process lifetimes and receiver-origin crash/reopen evidence.

package activity

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type receiverCrashPool struct {
	*sql.DB
	mode, generation, runID string
}
type receiverCrashTx struct {
	*sql.Tx
	pool *receiverCrashPool
}

func (p *receiverCrashPool) GetDBConn() (*sql.DB, error) { return p.DB, nil }
func (p *receiverCrashPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &receiverCrashTx{tx, p}, nil
}
func (p *receiverCrashPool) exit() {
	fmt.Printf("RECEIVER_PROCESS:%s|%s\n", p.generation, p.runID)
	_ = os.Stdout.Sync()
	os.Exit(0)
}
func (tx *receiverCrashTx) Commit() error {
	var state string
	_ = tx.QueryRow("SELECT executor_state FROM runtime_receipts WHERE run_id = ?", tx.pool.runID).Scan(&state)
	if state == "success" && tx.pool.mode == "before-terminal-commit" {
		tx.pool.exit()
	}
	err := tx.Tx.Commit()
	if err == nil && ((state == "accepted" && tx.pool.mode == "accepted-before-registration") || (state == "success" && tx.pool.mode == "after-terminal-commit")) {
		tx.pool.exit()
	}
	return err
}
func TestRuntimeReceiverProcessChild(t *testing.T) {
	mode := os.Getenv("RECEIVER_PROCESS_MODE")
	if mode == "" {
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	// Agent initialization runs exactly the same shipped 003 migration schema.
	db, err := database.Initialize(ctx, sqlitestore.URL(os.Getenv("RECEIVER_PROCESS_DB")), database.MigrationOptions{})
	require.NoError(t, err)
	svc := NewActivityService(db, nil)
	generation, err := NewRuntimeService(svc, &b1ResolverInternal{}).InitializeGeneration(ctx)
	require.NoError(t, err)
	runID := os.Getenv("RECEIVER_PROCESS_RUN")
	command := b3ProcessCommand(generation, runID, "")
	pool, err := db.DB.DB()
	require.NoError(t, err)
	wrapper := &receiverCrashPool{DB: pool, mode: mode, generation: generation, runID: runID}
	session := db.Session(&gorm.Session{NewDB: true})
	session.Config.ConnPool = wrapper
	session.Statement.ConnPool = wrapper
	svc.db = &database.DB{DB: session}
	runtime := NewRuntimeService(svc, &b1ResolverInternal{}, RuntimeLocalConfig{ApplicationContext: ctx, Store: command.Store,
		Executor: b2ExecutorFunc(func(context.Context, RuntimeCommand, io.Writer) error {
			require.NoError(t, os.WriteFile(os.Getenv("RECEIVER_PROCESS_COUNTER"), []byte("1"), 0600))
			if mode == "executing" {
				wrapper.exit()
			}
			return nil
		})})
	receiver, err := NewRuntimeReceiver(runtime)
	require.NoError(t, err)
	_, err = receiver.Execute(ctx, receiverEnvelope(t, command))
	require.NoError(t, err)
	<-ctx.Done()
	t.Fatal("child did not reach crash checkpoint")
}
func receiverProcessRun(t *testing.T, mode, path, runID, counter string) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestRuntimeReceiverProcessChild$", "-test.v")
	cmd.Env = append(os.Environ(), "RECEIVER_PROCESS_MODE="+mode, "RECEIVER_PROCESS_DB="+path, "RECEIVER_PROCESS_RUN="+runID, "RECEIVER_PROCESS_COUNTER="+counter)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	for _, line := range strings.Split(string(output), "\n") {
		if marker, ok := strings.CutPrefix(line, "RECEIVER_PROCESS:"); ok {
			parts := strings.Split(marker, "|")
			require.Len(t, parts, 2)
			require.Equal(t, runID, parts[1])
			return parts[0]
		}
	}
	t.Fatalf("no process checkpoint: %s", output)
	return ""
}
func TestRuntimeReceiverProcessCrashReopen(t *testing.T) {
	for _, mode := range []string{"accepted-before-registration", "executing", "before-terminal-commit", "after-terminal-commit"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path, counter := filepath.Join(dir, "receiver.db"), filepath.Join(dir, "effects")
			runID := uuid.NewString()
			generation := receiverProcessRun(t, mode, path, runID, counter)
			wantEffects := 1
			if mode == "accepted-before-registration" {
				wantEffects = 0
			}
			require.Equal(t, wantEffects, b3ProcessCounter(t, counter))
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			db, err := database.Initialize(ctx, sqlitestore.URL(path), database.MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			command := b3ProcessCommand(generation, runID, "")
			var replay atomic.Int64
			svc := NewActivityService(db, nil)
			runtime := NewRuntimeService(svc, &b1ResolverInternal{}, RuntimeLocalConfig{ApplicationContext: ctx, Store: command.Store,
				Executor: b2ExecutorFunc(func(context.Context, RuntimeCommand, io.Writer) error { replay.Add(1); return nil })})
			receiver, err := NewRuntimeReceiver(runtime)
			require.NoError(t, err)
			var own RuntimeStoreGeneration
			require.NoError(t, db.First(&own, "id = 1").Error)
			require.Equal(t, generation, own.Generation)
			before, activity := b3ProcessRow(t, db, runID)
			require.Equal(t, runID, before.RunID)
			require.Equal(t, "receipt", before.Kind)
			require.Equal(t, "0", before.EnvironmentID)
			require.Nil(t, before.DispatchedAt)
			runtimeCount(t, db.DB, &RuntimeKeyBinding{}, 0)
			if mode == "accepted-before-registration" {
				require.Equal(t, "accepted", *before.ExecutorState)
				require.Nil(t, before.ExecutorStartedAt)
				require.Equal(t, activitytypes.StatusQueued, activity.Status)
			}
			// Startup wiring is test-owned. Intake is deliberately withheld until it settles.
			count, err := runtime.SettleLocalStartup(ctx, command.Store)
			require.NoError(t, err)
			if mode == "after-terminal-commit" {
				require.Zero(t, count)
			} else {
				require.EqualValues(t, 1, count)
			}
			after, activity := b3ProcessRow(t, db, runID)
			if mode == "after-terminal-commit" {
				require.Equal(t, before, after)
				require.Equal(t, activitytypes.StatusSuccess, activity.Status)
			} else {
				require.Equal(t, "failed", *after.ExecutorState)
				require.Equal(t, string(activitytypes.CodeRuntimeInterrupted), *after.ExecutorResultCode)
				require.Equal(t, activitytypes.StatusFailed, activity.Status)
			}
			require.NotNil(t, after.SettledAt)
			require.Empty(t, runtime.owners)
			require.Empty(t, svc.slotReleases)
			b3ProcessFinalCount(t, db, runID)
			result, err := receiver.Execute(ctx, receiverEnvelope(t, command))
			require.NoError(t, err)
			require.Equal(t, activitytypes.RuntimeAnswerTerminal, result.State)
			query, err := receiver.Query(ctx, result.Identity)
			require.NoError(t, err)
			require.Equal(t, result, query)
			require.Zero(t, replay.Load())
			require.Equal(t, wantEffects, b3ProcessCounter(t, counter))
			count, err = runtime.SettleLocalStartup(ctx, command.Store)
			require.NoError(t, err)
			require.Zero(t, count)
			b3ProcessFinalCount(t, db, runID)
		})
	}
}
