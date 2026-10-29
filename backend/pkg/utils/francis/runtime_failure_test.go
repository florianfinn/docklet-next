// Source: docklet-next/issues/245
// Revision: 30d80ec7904e6faeeea28a36914e9d5a30b9d2da
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original actor-host loss report before a blocked teardown.

package francis

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/italypaleale/francis/actor"
	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/host/local"
	"github.com/stretchr/testify/require"
)

// holdingActor keeps one invocation in flight and ignores its context, so the
// host cannot finish halting actors until the test releases it.
type holdingActor struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *holdingActor) Invoke(context.Context, string, actor.Envelope) (any, error) {
	a.once.Do(func() { close(a.entered) })
	<-a.release
	return nil, nil
}

func TestExpiredHostRegistrationFailsAppBeforeBlockedTeardown(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	runtime, err := New(sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), "test-encryption-key", "test-instance", freePortInternal(t), local.WithHostHealthCheckDeadline(6*time.Second))
	require.NoError(t, err)
	holder := &holdingActor{entered: make(chan struct{}), release: make(chan struct{})}
	released := sync.OnceFunc(func() { close(holder.release) })
	t.Cleanup(released)
	require.NoError(t, runtime.RegisterActor("holding", func(string, *actor.Service) actor.Actor { return holder }))
	failures := make(chan error, 2)
	require.NoError(t, runtime.Start(ctx, ctx, func(err error) { failures <- err }))
	t.Cleanup(func() { released(); _ = runtime.Stop(context.WithoutCancel(ctx)) })

	go func() { _, _ = runtime.Service().Invoke(ctx, "holding", "held", "hold", nil) }()
	select {
	case <-holder.entered:
	case <-ctx.Done():
		t.Fatal("holding invocation never started")
	}
	runtime.mu.Lock()
	writer := runtime.writer
	runtime.mu.Unlock()
	_, err = writer.ExecContext(ctx, `DELETE FROM `+TablePrefix+`_hosts`)
	require.NoError(t, err)

	var reported error
	select {
	case reported = <-failures:
	case <-time.After(10 * time.Second):
	}
	released()
	require.NotNil(t, reported, "actor-host loss reaches the application while actor teardown is still blocked")
	require.True(t, errors.Is(reported, components.ErrHostUnregistered), "reported cause: %v", reported)
}

// A service failure before the host is published belongs to startup and is
// never reported; after publication it is reported once, unless cancelled.
func TestHostFailureReportsOnlyAfterPublicationAndOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var reported []error
	failure := &hostFailureInternal{ctx: ctx, report: func(err error) { reported = append(reported, err) }}
	failure.fail(errors.New("before publication"))
	require.Empty(t, reported, "a failure before publication is a start error")
	failure.armed.Store(true)
	first := errors.New("first")
	failure.fail(first)
	failure.fail(errors.New("second"))
	require.Equal(t, []error{first}, reported)

	cancelled := &hostFailureInternal{ctx: ctx, report: func(err error) { reported = append(reported, err) }}
	cancelled.armed.Store(true)
	cancel()
	cancelled.fail(errors.New("after cancel"))
	require.Equal(t, []error{first}, reported, "shutdown is not a failure")
}
