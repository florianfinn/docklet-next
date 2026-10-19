// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Runtime capture concurrency, drain, error and reuse regression proofs.

package activity

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

type runtimeAppenderInternal struct {
	recordingAppender
	append func(context.Context, string, []AppendMessageRequest) error
}

func (a *runtimeAppenderInternal) AppendMessages(ctx context.Context, id string, req []AppendMessageRequest) ([]activitytypes.Message, error) {
	if a.append != nil {
		if err := a.append(ctx, id, req); err != nil {
			return nil, err
		}
	}
	return a.recordingAppender.AppendMessages(ctx, id, req)
}
func runtimeWriterAwait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("capture phase barrier not reached")
	}
}
func runtimeWriterPending(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
		t.Fatal("capture joined before appender release")
	default:
	}
}

func TestRuntimeWriterDrainBackpressureCancelAndTail(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	a := &runtimeAppenderInternal{}
	var first sync.Once
	a.append = func(context.Context, string, []AppendMessageRequest) error {
		first.Do(func() { close(entered) })
		<-release
		return nil
	}
	w, err := NewRuntimeWriter(ctx, a, "example-run", "deploy")
	require.NoError(t, err)
	_, err = w.Write([]byte("first\n"))
	require.NoError(t, err)
	runtimeWriterAwait(t, entered)
	for i := 0; i < writerAppendQueueSize; i++ {
		_, err = w.Write(fmt.Appendf(nil, "line %d\n", i))
		require.NoError(t, err)
	}
	require.Len(t, w.queueCh, 128)
	cancel() // Accepted output must survive execution cancellation.
	wrote := make(chan struct{})
	go func() {
		_, e := w.Write([]byte("last\ntail"))
		if e != nil {
			t.Error(e)
		}
		close(wrote)
	}()
	// Observe extraction before Finish so this producer is known accepted,
	// not merely waiting for ingress. A full queue holds it at enqueue.
	deadline := time.After(5 * time.Second)
	for {
		w.mu.Lock()
		acceptedTail := string(w.buffer) == "tail"
		w.mu.Unlock()
		if acceptedTail {
			break
		}
		select {
		case <-deadline:
			t.Fatal("writer did not reach enqueue")
		default:
			runtime.Gosched()
		}
	}
	wait, stop := context.WithCancel(t.Context())
	stop()
	require.ErrorIs(t, w.Finish(wait), context.Canceled)
	runtimeWriterPending(t, w.Done())
	runtimeWriterPending(t, wrote)
	unblock()
	runtimeWriterAwait(t, wrote)
	finishCtx, finishCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer finishCancel()
	require.NoError(t, w.Finish(finishCtx))
	runtimeWriterAwait(t, w.Done())
	require.NoError(t, w.Finish(finishCtx))
	n, err := w.Write([]byte("too late\n"))
	require.Zero(t, n)
	require.ErrorIs(t, err, ErrWriterFinished)
	messages, batches := a.recorded()
	require.Len(t, messages, 131)
	require.Equal(t, "first", messages[0].Output.Text)
	for i := 0; i < 128; i++ {
		require.Equal(t, fmt.Sprintf("line %d", i), messages[i+1].Output.Text)
	}
	require.Equal(t, "last", messages[129].Output.Text)
	require.Equal(t, "tail", messages[130].Output.Text)
	for _, size := range batches {
		require.LessOrEqual(t, size, 32)
	}
}

func TestRuntimeWriterFirstErrorSurvivesSuccess(t *testing.T) {
	failure := errors.New("fixture append failed")
	a := &runtimeAppenderInternal{}
	var calls atomic.Int64
	a.append = func(context.Context, string, []AppendMessageRequest) error {
		if calls.Add(1) == 1 {
			return failure
		}
		return nil
	}
	w, err := NewRuntimeWriter(t.Context(), a, "example-run", "")
	require.NoError(t, err)
	_, err = w.Write([]byte("lost\n"))
	require.NoError(t, err)
	w.Flush() // Acknowledgment must already retain the failed append.
	w.mu.Lock()
	retained := w.runtime.firstError
	w.mu.Unlock()
	require.ErrorIs(t, retained, failure)
	_, err = w.Write([]byte("persisted\npartial"))
	require.NoError(t, err)
	require.ErrorIs(t, w.Finish(t.Context()), failure)
	require.ErrorIs(t, w.Finish(t.Context()), failure)
	messages, _ := a.recorded()
	require.Len(t, messages, 2)
	require.Equal(t, "partial", messages[1].Output.Text)
	require.GreaterOrEqual(t, calls.Load(), int64(2))
}

func TestRuntimeWriterAppendContextAndWaitTimeout(t *testing.T) {
	type runKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), runKey{}, "example-run"))
	cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer func() { once.Do(func() { close(release) }) }()
	observed := make(chan context.Context, 1)
	a := &runtimeAppenderInternal{append: func(ctx context.Context, id string, _ []AppendMessageRequest) error {
		observed <- ctx
		close(entered)
		<-release
		return nil
	}}
	w, err := NewRuntimeWriter(ctx, a, "example-run", "")
	require.NoError(t, err)
	_, err = w.Write([]byte("line\n"))
	require.NoError(t, err)
	runtimeWriterAwait(t, entered)
	appendCtx := <-observed
	require.NoError(t, appendCtx.Err())
	require.Equal(t, "example-run", appendCtx.Value(runKey{}))
	deadline, ok := appendCtx.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(deadline), 10*time.Second)
	wait, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stop()
	require.ErrorIs(t, w.Finish(wait), context.DeadlineExceeded)
	runtimeWriterPending(t, w.Done())
	once.Do(func() { close(release) })
	require.NoError(t, w.Finish(t.Context()))
}

func TestRuntimeWriterConcurrentWriteFlushFinish(t *testing.T) {
	a := &runtimeAppenderInternal{}
	w, err := NewRuntimeWriter(t.Context(), a, "example-run", "")
	require.NoError(t, err)
	var wg sync.WaitGroup
	var accepted atomic.Int64
	start := make(chan struct{})
	for i := 0; i < 40; i++ {
		wg.Go(func() {
			<-start
			_, e := w.Write([]byte("line\n"))
			if e == nil {
				accepted.Add(1)
			} else if !errors.Is(e, ErrWriterFinished) {
				t.Error(e)
			}
		})
	}
	for i := 0; i < 8; i++ {
		wg.Go(func() { <-start; w.Flush() })
	}
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			<-start
			if e := w.Finish(t.Context()); e != nil {
				t.Error(e)
			}
		})
	}
	close(start)
	wg.Wait()
	require.NoError(t, w.Finish(t.Context()))
	messages, _ := a.recorded()
	require.Len(t, messages, int(accepted.Load()))
}

func TestRuntimeWriterReuseSeparatesRunsAndModes(t *testing.T) {
	a := &runtimeAppenderInternal{}
	w, err := NewRuntimeWriter(t.Context(), a, "example-first", "")
	require.NoError(t, err)
	require.Same(t, w, NewWriter(t.Context(), a, "example-first", w, "nested"))
	legacyCtx, stop := context.WithCancel(t.Context())
	defer stop()
	other := NewWriter(legacyCtx, a, "example-second", w, "")
	require.NotSame(t, w, other)
	_, err = other.Write([]byte("second\n"))
	require.NoError(t, err)
	other.(*Writer).Flush()
	require.NoError(t, w.Finish(t.Context()))
	messages, _ := a.recorded()
	require.Len(t, messages, 1)
	require.ErrorIs(t, other.(*Writer).Finish(t.Context()), ErrWriterNotRuntime)
	require.Nil(t, other.(*Writer).Done())
	// A runtime constructor always owns a fresh capture, even alongside legacy.
	next, err := NewRuntimeWriter(t.Context(), a, "example-third", "")
	require.NoError(t, err)
	require.NotSame(t, w, next)
	require.NoError(t, next.Finish(t.Context()))
}
