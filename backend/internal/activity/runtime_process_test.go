// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Separate executable lifetimes, actual COMMIT crash boundaries and safe snapshot fixture.

package activity

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func b3ProcessCommand(generation, runID, key string) RuntimeCommand {
	return RuntimeCommand{Store: RuntimeStoreBinding{StoreGeneration: generation, ExecutorGeneration: generation,
		ManagerID: "example-manager", RegistrationID: "example-registration", EnvironmentID: "0"},
		ProposedRunID: runID, Key: key, Actor: usertypes.Actor{ID: "example-owner", Username: "Operator"},
		Operation: RuntimeDeployProject, Target: RuntimeTarget{Kind: "stack", ID: "example-process-stack"}}
}

// This child is a separate executable holding a real InitializeHub lifetime lock.
// os.Exit skips Go cleanup at each named persistence boundary.
func TestRuntimeB3ProcessChild(t *testing.T) {
	mode := os.Getenv("B3_PROCESS_MODE")
	if mode == "" {
		return
	}
	url := os.Getenv("B3_PROCESS_URL")
	db, err := database.InitializeHub(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	service := NewActivityService(db, nil)
	runtime := NewRuntimeService(service, &b1ResolverInternal{})
	generation, err := runtime.InitializeGeneration(t.Context())
	require.NoError(t, err)
	runID, key := os.Getenv("B3_PROCESS_RUN"), os.Getenv("B3_PROCESS_KEY")
	if mode == "hold-lock" {
		require.NoError(t, os.WriteFile(os.Getenv("B3_PROCESS_READY"), []byte(generation), 0600))
		for {
			if _, err := os.Stat(os.Getenv("B3_PROCESS_RELEASE")); err == nil {
				os.Exit(0)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	c := b3ProcessCommand(generation, runID, key)
	if mode == "foreign-identity" {
		c.Store.ManagerID = "example-other-manager"
	}
	admitted, err := runtime.Admit(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, runID, admitted.ActivityID)
	if mode != "admitted" {
		id := b1Identity(t, c, runID)
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			now := runtimeTimeInternal(time.Now())
			_, err := runtimeLocalAcceptTxInternal(tx, now, id)
			if err != nil || mode == "accepted" {
				return err
			}
			_, err = runtimeLocalExecutingTxInternal(tx, now, id)
			return err
		}))
	}
	// Test-owned durable effect-start record is outside the receipt database.
	// Startup must never append another record or execute the controlled port.
	if mode == "executing" || mode == "before-terminal-commit" || mode == "after-terminal-commit" {
		require.NoError(t, os.WriteFile(os.Getenv("B3_PROCESS_COUNTER"), []byte("1"), 0600))
	}
	if mode == "before-terminal-commit" || mode == "after-terminal-commit" {
		runCtx := context.WithValue(t.Context(), gateRunContextKey{}, runID)
		err = db.WithContext(runCtx).Transaction(func(tx *gorm.DB) error {
			now := runtimeTimeInternal(time.Now())
			row, err := runtimeReadTxInternal(tx, b1Identity(t, c, runID))
			if err != nil {
				return err
			}
			result := runtimeLocalResultInternal{state: "success", code: activitytypes.CodeActivitySuccess,
				message: "Runtime workflow completed", startedAt: row.ExecutorStartedAt, settledAt: now}
			_, final, err := runtimeCompleteTxInternal(tx, now, b1Identity(t, c, runID), result)
			if err != nil {
				return err
			}
			require.NotNil(t, final)
			if mode == "before-terminal-commit" {
				fmt.Printf("B3_PROCESS:%s|%s|%s\n", generation, runID, key)
				os.Stdout.Sync()
				os.Exit(0)
			}
			return nil
		})
		require.NoError(t, err)
	}
	fmt.Printf("B3_PROCESS:%s|%s|%s\n", generation, runID, key)
	os.Stdout.Sync()
	os.Exit(0)
}

func b3ProcessRun(t *testing.T, mode, url, runID, key, counter string) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestRuntimeB3ProcessChild$", "-test.v")
	cmd.Env = append(os.Environ(), "B3_PROCESS_MODE="+mode, "B3_PROCESS_URL="+url, "B3_PROCESS_RUN="+runID,
		"B3_PROCESS_KEY="+key, "B3_PROCESS_COUNTER="+counter)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	for _, line := range strings.Split(string(output), "\n") {
		if marker, ok := strings.CutPrefix(line, "B3_PROCESS:"); ok {
			parts := strings.Split(marker, "|")
			require.Len(t, parts, 3)
			require.Equal(t, runID, parts[1])
			require.Equal(t, key, parts[2])
			return parts[0]
		}
	}
	t.Fatalf("child exited without reaching persistence boundary: %s", output)
	return ""
}
func b3ProcessOpen(t *testing.T, path string) (*database.DB, *RuntimeService) {
	t.Helper()
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	return db, NewRuntimeService(NewActivityService(db, nil), &b1ResolverInternal{})
}
func b3ProcessCounter(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	require.Equal(t, "1", string(data))
	return 1
}
func b3ProcessRow(t *testing.T, db *database.DB, runID string) (RuntimeReceipt, Activity) {
	t.Helper()
	var row RuntimeReceipt
	var activity Activity
	require.NoError(t, db.First(&row, "run_id = ?", runID).Error)
	require.NoError(t, db.First(&activity, "id = ?", runID).Error)
	return row, activity
}
func b3ProcessFinalCount(t *testing.T, db *database.DB, runID string) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ? AND code IN ?", runID,
		[]string{"activity.success", "runtime.interrupted"}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestRuntimeB3ProcessCrashBoundaries(t *testing.T) {
	for _, mode := range []string{"admitted", "accepted", "executing", "before-terminal-commit", "after-terminal-commit"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "process.db")
			url := sqlitestore.URL(path)
			counter := filepath.Join(t.TempDir(), "effect-started")
			runID, key := uuid.NewString(), uuid.NewString()
			generation := b3ProcessRun(t, mode, url, runID, key, counter)
			wantCounter := 0
			if mode == "executing" || strings.Contains(mode, "terminal") {
				wantCounter = 1
			}
			require.Equal(t, wantCounter, b3ProcessCounter(t, counter))
			db, runtime := b3ProcessOpen(t, path)
			defer db.Close()
			c := b3ProcessCommand(generation, runID, key)
			// The old process has exited. Before the explicit startup entry, a new
			// deliberate key still sees the durable held gate.
			next := c
			next.Key, next.ProposedRunID = uuid.NewString(), uuid.NewString()
			next.Operation = RuntimeDownProject
			if mode != "after-terminal-commit" {
				_, err := runtime.Admit(t.Context(), next)
				var busy *GateConflict
				require.ErrorAs(t, err, &busy)
			}
			count, err := runtime.SettleLocalStartup(t.Context(), c.Store)
			require.NoError(t, err)
			if mode == "after-terminal-commit" {
				require.Zero(t, count)
			} else {
				require.EqualValues(t, 1, count)
			}
			row, activity := b3ProcessRow(t, db, runID)
			if mode == "after-terminal-commit" {
				require.Equal(t, "success", *row.ExecutorState)
				require.Equal(t, activitytypes.StatusSuccess, activity.Status)
			} else {
				require.Equal(t, "failed", *row.ExecutorState)
				require.Equal(t, string(activitytypes.CodeRuntimeInterrupted), *row.ExecutorResultCode)
				require.Equal(t, activitytypes.StatusFailed, activity.Status)
			}
			require.NotNil(t, row.SettledAt)
			b3ProcessFinalCount(t, db, runID)
			count, err = runtime.SettleLocalStartup(t.Context(), c.Store)
			require.NoError(t, err)
			require.Zero(t, count)
			b3ProcessFinalCount(t, db, runID)
			require.Equal(t, wantCounter, b3ProcessCounter(t, counter), "startup must not replay an effect")
			_, err = runtime.activity.DeleteHistory(t.Context(), "0")
			require.NoError(t, err)
			runtime.activity.clock = func() time.Time { return time.Now().Add(11 * time.Minute) }
			runtime.resolver = &b1ResolverInternal{resolve: func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error) {
				return RuntimeResolvedTarget{}, fmt.Errorf("offline")
			}}
			retained, err := runtime.Admit(t.Context(), c)
			require.NoError(t, err)
			require.Equal(t, runID, retained.ActivityID)
			require.Equal(t, activity.Status, retained.Activity.Status)
			require.Equal(t, wantCounter, b3ProcessCounter(t, counter))
			var activityCount int64
			require.NoError(t, db.Model(&Activity{}).Where("id = ?", runID).Count(&activityCount).Error)
			require.Zero(t, activityCount)
			runtime.resolver = &b1ResolverInternal{}
			admitted, err := runtime.Admit(t.Context(), next)
			require.NoError(t, err)
			require.Equal(t, activitytypes.CodeRunStarted, admitted.Code)
			require.NotEqual(t, runID, admitted.ActivityID)
			require.Equal(t, wantCounter, b3ProcessCounter(t, counter))
		})
	}
}

func TestRuntimeB3ProcessForeignIdentityHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foreign.db")
	runID, key := uuid.NewString(), uuid.NewString()
	generation := b3ProcessRun(t, "foreign-identity", sqlitestore.URL(path), runID, key, filepath.Join(t.TempDir(), "unused"))
	db, runtime := b3ProcessOpen(t, path)
	defer db.Close()
	var before RuntimeReceipt
	require.NoError(t, db.First(&before, "run_id = ?", runID).Error)
	current := b3ProcessCommand(generation, runID, key).Store
	count, err := runtime.SettleLocalStartup(t.Context(), current)
	require.NoError(t, err)
	require.Zero(t, count)
	var after RuntimeReceipt
	require.NoError(t, db.First(&after, "run_id = ?", runID).Error)
	require.Equal(t, before, after)
	var finalCount int64
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ? AND code = ?", runID, "runtime.interrupted").Count(&finalCount).Error)
	require.Zero(t, finalCount)
}

func TestRuntimeB3ProcessLifetimeLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lock.db")
	url := sqlitestore.URL(path)
	exe, err := os.Executable()
	require.NoError(t, err)
	ready, release := filepath.Join(dir, "ready"), filepath.Join(dir, "release")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestRuntimeB3ProcessChild$")
	cmd.Env = append(os.Environ(), "B3_PROCESS_MODE=hold-lock", "B3_PROCESS_URL="+url,
		"B3_PROCESS_READY="+ready, "B3_PROCESS_RELEASE="+release)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	waited := false
	defer func() {
		if !waited {
			_ = os.WriteFile(release, nil, 0600)
			_ = cmd.Wait()
		}
	}()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case <-deadline:
			data, _ := io.ReadAll(stdout)
			t.Fatalf("child lock not ready: %s", data)
		case <-time.After(10 * time.Millisecond):
		}
	}
	_, err = database.InitializeHub(t.Context(), url, database.MigrationOptions{})
	require.ErrorContains(t, err, "hub.already_running")
	require.NoError(t, os.WriteFile(release, nil, 0600))
	require.NoError(t, cmd.Wait())
	waited = true
	db, err := database.InitializeHub(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Close())
}

func TestRuntimeB3ProcessRestoredAdmittedSnapshot(t *testing.T) {
	dir := t.TempDir()
	path, snapshot := filepath.Join(dir, "restore.db"), filepath.Join(dir, "snapshot.db")
	counter := filepath.Join(dir, "effect-started")
	runID, key := uuid.NewString(), uuid.NewString()
	generation := b3ProcessRun(t, "admitted", sqlitestore.URL(path), runID, key, counter)
	db, _ := b3ProcessOpen(t, path)
	// VACUUM INTO produces a consistent standalone SQLite image even when the
	// source uses WAL. Both pools and the lifetime owner close before replacement.
	require.NoError(t, db.Exec("VACUUM INTO ?", snapshot).Error)
	require.NoError(t, db.Close())
	require.NoError(t, os.WriteFile(counter, []byte("1"), 0600)) // possible effect after backup
	image, err := os.ReadFile(snapshot)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, image, 0600))
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	reopened, runtime := b3ProcessOpen(t, path)
	defer reopened.Close()
	c := b3ProcessCommand(generation, runID, key)
	count, err := runtime.SettleLocalStartup(t.Context(), c.Store)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	row, activity := b3ProcessRow(t, reopened, runID)
	require.Equal(t, "failed", *row.ExecutorState)
	require.Equal(t, string(activitytypes.CodeRuntimeInterrupted), *row.ExecutorResultCode)
	require.Equal(t, "Runtime workflow interrupted; effects unverified", *row.ExecutorError)
	require.Nil(t, row.ExecutorStartedAt)
	require.Equal(t, activitytypes.StatusFailed, activity.Status)
	b3ProcessFinalCount(t, reopened, runID)
	require.Equal(t, 1, b3ProcessCounter(t, counter), "restored admission cannot prove no later effect")
}
