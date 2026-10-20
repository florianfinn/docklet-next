// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRuntimeWindowDurableBoundedAndPruned(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	ctx := context.WithValue(t.Context(), gateRunContextKey{}, e.Identity.RunID)
	appendLine := func(text string) {
		_, err := f.store.svc.AppendMessages(ctx, e.Identity.RunID, []AppendActivityMessageRequest{{Output: &activitytypes.OutputLine{Timestamp: time.Now(), Stream: "stdout", Text: text}}})
		require.NoError(t, err)
	}
	appendLine("first")
	answer, err := f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Equal(t, uint64(1), answer.Detail.Output[0].OutputSeq)
	// Concurrent service appends allocate under the same atomic durable writer.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { appendLine("concurrent") })
	}
	wg.Wait()
	answer, err = f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Len(t, answer.Detail.Output, 9)
	for i, line := range answer.Detail.Output {
		require.Equal(t, uint64(i+1), line.OutputSeq)
	}
	appendLine(strings.Repeat("x", 300<<10))
	answer, err = f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.True(t, answer.Detail.OutputTruncated)
	require.Equal(t, uint64(10), answer.Detail.RetainedStartSeq)
	encoded, err := json.Marshal(answer.Detail.Output)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), 256<<10)
	// A disabled output batch still consumes sequence and records actual loss.
	require.NoError(t, f.store.svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model Activity
		if err := tx.First(&model, "id = ?", e.Identity.RunID).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		if err := runtimeAppendOutputTxInternal(tx, &model, updates, []activitytypes.OutputLine{{Timestamp: time.Now(), Text: "disabled"}}, false); err != nil {
			return err
		}
		return tx.Model(&model).UpdateColumns(updates).Error
	}))
	appendLine("after-disabled")
	answer, err = f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Equal(t, uint64(12), answer.Detail.Output[len(answer.Detail.Output)-1].OutputSeq)
	p.unblock()
	receiverWait(t, f, e)
	require.NoError(t, f.store.svc.db.Where("id = ?", e.Identity.RunID).Delete(&Activity{}).Error)
	answer, err = f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Empty(t, answer.Detail.Output)
	require.True(t, answer.Detail.OutputTruncated)
	require.Equal(t, uint64(13), answer.Detail.RetainedStartSeq)
	require.Equal(t, activitytypes.RuntimeAnswerTerminal, answer.State)
}
func TestRuntimeWindowRollbackNeverConsumesCursor(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER window_fault BEFORE UPDATE OF output ON activities BEGIN SELECT RAISE(ABORT,'fixture'); END").Error)
	ctx := context.WithValue(t.Context(), gateRunContextKey{}, e.Identity.RunID)
	req := []AppendActivityMessageRequest{{Output: &activitytypes.OutputLine{Timestamp: time.Now(), Text: "once"}}}
	_, err := f.store.svc.AppendMessages(ctx, e.Identity.RunID, req)
	require.Error(t, err)
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER window_fault").Error)
	_, err = f.store.svc.AppendMessages(ctx, e.Identity.RunID, req)
	require.NoError(t, err)
	answer, err := f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Len(t, answer.Detail.Output, 1)
	require.Equal(t, uint64(1), answer.Detail.Output[0].OutputSeq)
	p.unblock()
	receiverWait(t, f, e)
}

func TestRuntimeWindowFailedAppendNeverClaimsLosslessEmpty(t *testing.T) {
	f := receiverNew(t, func(_ context.Context, _ RuntimeCommand, out io.Writer) error {
		_, err := out.Write([]byte("lost line\n"))
		return err
	})
	f.service.outputAppender = &b2OutputAppender{ActivityService: f.store.svc, before: func(context.Context, string, []activitylib.AppendMessageRequest) error {
		return errors.New("synthetic output failure")
	}}
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	snapshot := receiverWait(t, f, e)
	require.Equal(t, activitytypes.StatusFailed, snapshot.Status)
	answer, err := f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Empty(t, answer.Detail.Output)
	require.True(t, answer.Detail.OutputTruncated)
}

func TestRuntimeWindowDisabledPreferenceAndDatabaseReopen(t *testing.T) {
	f := receiverNew(t, func(_ context.Context, _ RuntimeCommand, out io.Writer) error {
		_, err := out.Write([]byte("disabled output\n"))
		return err
	})
	preferences, err := settings.NewSettingsService(t.Context(), f.store.svc.db)
	require.NoError(t, err)
	require.NoError(t, preferences.SetBoolSetting(t.Context(), "keepActionOutput", false))
	f.store.svc.settings = preferences
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	receiverWait(t, f, e)
	answer, err := f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Empty(t, answer.Detail.Output)
	require.True(t, answer.Detail.OutputTruncated)
	require.Equal(t, uint64(2), answer.Detail.RetainedStartSeq)
	require.NoError(t, preferences.Stop(context.WithoutCancel(t.Context())))
	require.NoError(t, f.store.svc.db.Close())
	reopened, err := database.InitializeHub(t.Context(), sqlitestore.URL(f.store.path), database.MigrationOptions{})
	require.NoError(t, err)
	defer reopened.Close()
	var calls int
	runtime := NewRuntimeService(NewActivityService(reopened, nil), &b1ResolverInternal{}, RuntimeLocalConfig{ApplicationContext: t.Context(), Store: f.receiver.store, Executor: b2ExecutorFunc(func(context.Context, RuntimeCommand, io.Writer) error { calls++; return nil })})
	receiver, err := NewRuntimeReceiver(runtime)
	require.NoError(t, err)
	after, err := receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Equal(t, answer, after)
	duplicate, err := receiver.Execute(t.Context(), e)
	require.NoError(t, err)
	require.Equal(t, answer, duplicate)
	require.Zero(t, calls)
}

func TestRuntimeWindowLargeActualProducerHasBoundedPositiveDetail(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	f := receiverNew(t, func(_ context.Context, _ RuntimeCommand, out io.Writer) error {
		_, err := out.Write([]byte(strings.Repeat("x", 100<<10) + "\n"))
		out.(interface{ Flush() }).Flush()
		close(entered)
		<-release
		return err
	})
	t.Cleanup(unblock)
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	b1Await(t, entered)
	answer, err := f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Len(t, answer.Detail.Output, 1)
	require.Len(t, answer.Detail.Output[0].Text, 100<<10)
	require.Len(t, answer.Detail.Activity.LatestMessage, 8192)
	require.LessOrEqual(t, len(answer.Detail.Activity.LatestMessage), 16<<10)
	unblock()
	receiverWait(t, f, e)
}
