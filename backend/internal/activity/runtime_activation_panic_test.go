// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Actual subprocess panic lifetime and durable interrupted startup proof.

package activity

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationPanicChild(t *testing.T) {
	if os.Getenv("ACTIVATION_PANIC_CHILD") == "" {
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	db, err := database.InitializeHub(ctx, os.Getenv("ACTIVATION_PANIC_URL"), database.MigrationOptions{})
	require.NoError(t, err)
	svc := NewActivityService(db, nil)
	bootstrap := NewRuntimeService(svc, &b1ResolverInternal{})
	generation, err := bootstrap.InitializeGeneration(ctx)
	require.NoError(t, err)
	command := b3ProcessCommand(generation, os.Getenv("ACTIVATION_PANIC_RUN"), uuid.NewString())
	runtime := NewRuntimeService(svc, &b1ResolverInternal{}, RuntimeLocalConfig{ApplicationContext: ctx, Store: command.Store,
		Executor: b2ExecutorFunc(func(context.Context, RuntimeCommand, io.Writer) error {
			childStarted := make(chan struct{})
			go func() { close(childStarted); select {} }()
			<-childStarted
			panic("synthetic-runtime-child-completion-unknown")
		})})
	_, err = runtime.Admit(ctx, command)
	require.NoError(t, err)
	_, err = runtime.WaitLocal(ctx, b2Ref(command, command.ProposedRunID))
	require.NoError(t, err, "a panic must not fabricate a settled result")
	t.Fatal("worker panic did not terminate process")
}

func TestRuntimeActivationPanicPreservesExclusionUntilProcessEnds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panic.db")
	runID := uuid.NewString()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestRuntimeActivationPanicChild$", "-test.v")
	child.Env = append(os.Environ(), "ACTIVATION_PANIC_CHILD=1", "ACTIVATION_PANIC_URL="+sqlitestore.URL(path), "ACTIVATION_PANIC_RUN="+runID)
	output, err := child.CombinedOutput()
	var exited *exec.ExitError
	require.ErrorAs(t, err, &exited)
	require.Equal(t, 2, exited.ExitCode(), string(output))
	require.Contains(t, string(output), "panic: synthetic-runtime-child-completion-unknown")
	require.NotContains(t, string(output), "worker panic did not terminate")
	db, runtime := b3ProcessOpen(t, path)
	defer db.Close()
	row, model := b3ProcessRow(t, db, runID)
	require.Nil(t, row.SettledAt, "panic cannot prove that known children joined")
	require.Equal(t, "executing", *row.ExecutorState)
	require.Equal(t, activitytypes.StatusRunning, model.Status)
	var finals int64
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ? AND code IN ?", runID,
		[]string{"activity.success", "runtime.failed", "runtime.cancelled", "runtime.interrupted"}).Count(&finals).Error)
	require.Zero(t, finals)
	// Acquiring InitializeHub proves the prior executable and its children ended.
	count, err := runtime.SettleLocalStartup(ctx, runtimeStoreFromRowInternal(&row))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	row, model = b3ProcessRow(t, db, runID)
	require.Equal(t, "runtime.interrupted", *row.ExecutorResultCode)
	require.Equal(t, activitytypes.StatusFailed, model.Status)
	require.NotNil(t, row.SettledAt)
	b3ProcessFinalCount(t, db, runID)
}
