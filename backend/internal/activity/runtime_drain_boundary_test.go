// Source: docklet-next/docs/design/runtime-control.md
// Revision: c0c290cb4ce7bbba37fdbd6d4821692df0312738
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Executor-return, cancellation and actual output-budget classification evidence.

package activity

import (
	"context"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func runtimeDrainWaiting(inFinish bool) bool {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
		frames := strings.Split(stack, "\n")
		if len(frames) < 2 {
			continue
		}
		joinedWait := strings.Contains(frames[0], "[chan receive") && strings.Contains(frames[1], "runtimeFinishOutputInternal(")
		if strings.Contains(stack, "runtimeFinishOutputInternal(") && strings.Contains(stack, "(*RuntimeService).runLocalInternal(") && strings.Contains(stack, "(*Writer).Finish(") == inFinish && (inFinish || joinedWait) {
			return true
		}
	}
	return false
}

// This deliberately ignores the append context to model a late, successful drain.
// The real ActivityService still commits every accepted output line.
type runtimeLateAppender struct {
	*ActivityService
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *runtimeLateAppender) AppendMessages(ctx context.Context, id string, req []activitylib.AppendMessageRequest) ([]activitytypes.Message, error) {
	a.once.Do(func() { close(a.entered) })
	<-a.release
	return a.ActivityService.AppendMessages(context.WithoutCancel(ctx), id, req)
}

func TestRuntimeWorkerCancellationAfterExecutorReturnDuringDrain(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			// Construct the deadline after fixture setup so it cannot expire during setup.
			app, stop := context.WithCancel(t.Context())
			defer stop()
			f := b2Fixture(t, app, func(_ context.Context, _ RuntimeCommand, w io.Writer) error {
				_, err := io.WriteString(w, "accepted tail\n")
				return err
			})
			if mode == "deadline" {
				deadline, cancel := context.WithTimeout(app, 3*time.Second)
				defer cancel()
				f.service.localExecution.ApplicationContext = deadline
			}
			a := &runtimeLateAppender{ActivityService: f.store.svc, entered: make(chan struct{}), release: make(chan struct{})}
			f.service.outputAppender = a
			unblock := sync.OnceFunc(func() { close(a.release) })
			defer unblock()
			c := f.command()
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			b1Await(t, a.entered)
			require.Eventually(t, func() bool { return runtimeDrainWaiting(true) }, 2*time.Second, time.Millisecond, "executor must have returned before cancellation")
			owner := b2Owner(t, f, admitted.ActivityID)
			if mode == "cancel" {
				_, err = f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
				require.NoError(t, err)
			} else {
				b1Await(t, owner.ctx.Done())
				require.ErrorIs(t, owner.ctx.Err(), context.DeadlineExceeded)
			}
			b2Held(t, f, c, admitted.ActivityID)
			b2FinalCount(t, f, admitted.ActivityID, 0)
			select {
			case <-owner.attemptDone:
				t.Fatal("settled before output joined")
			default:
			}
			unblock()
			snapshot, err := b2Wait(t, f, c, admitted.ActivityID)
			require.NoError(t, err)
			if mode == "cancel" {
				require.Equal(t, activitytypes.StatusCancelled, snapshot.Status)
				require.Equal(t, string(activitytypes.CodeRuntimeCancelled), snapshot.MessageCode)
			} else {
				require.Equal(t, activitytypes.StatusFailed, snapshot.Status)
				require.Equal(t, string(activitytypes.CodeRuntimeInterrupted), snapshot.MessageCode)
			}
			require.Contains(t, snapshot.LatestMessage, "effects unverified")
			b2FinalCount(t, f, admitted.ActivityID, 1)
			require.False(t, owner.result.outputLost, "the accepted output eventually persisted")
			require.Len(t, runtimeBoundarySnapshot(t, f.store).Activities[0].Output, 1)
		})
	}
}

func TestRuntimeWorkerDrainBudgetExpiresBeforeSuccessfulJoin(t *testing.T) {
	f := b2Fixture(t, t.Context(), func(_ context.Context, _ RuntimeCommand, w io.Writer) error {
		_, err := io.WriteString(w, "late output\n")
		return err
	})
	a := &runtimeLateAppender{ActivityService: f.store.svc, entered: make(chan struct{}), release: make(chan struct{})}
	f.service.outputAppender = a
	unblock := sync.OnceFunc(func() { close(a.release) })
	defer unblock()
	c := f.command()
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	b1Await(t, a.entered)
	// Observe the actual post-budget Done join, not a sleep or a fabricated error.
	require.Eventually(t, func() bool { return runtimeDrainWaiting(false) }, runtimeOutputDrainWait+5*time.Second, 10*time.Millisecond)
	b2Held(t, f, c, admitted.ActivityID)
	b2FinalCount(t, f, admitted.ActivityID, 0)
	owner := b2Owner(t, f, admitted.ActivityID)
	select {
	case <-owner.attemptDone:
		t.Fatal("budget expiry released an unfinished drain")
	default:
	}
	unblock()
	snapshot, err := b2Wait(t, f, c, admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusFailed, snapshot.Status)
	require.Equal(t, string(activitytypes.CodeRuntimeFailed), snapshot.MessageCode)
	require.True(t, owner.result.outputLost, "wait-budget failure remains conservative evidence")
	require.Contains(t, snapshot.LatestMessage, "effects unverified")
	require.Len(t, runtimeBoundarySnapshot(t, f.store).Activities[0].Output, 1, "late success does not erase the earlier drain-budget failure")
	b2FinalCount(t, f, admitted.ActivityID, 1)
}
