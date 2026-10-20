// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

package sessionlife

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionRegistryReentrantConcurrentCloseAndIsolation(t *testing.T) {
	var closed atomic.Int32
	ctx := WithSession(t.Context(), "example-session", "example-user", func(context.Context) error { return nil })
	var leases []*Lease
	for range 2 {
		var l *Lease
		l, _ = Register(ctx, func() { l.Release(); closed.Add(1) })
		leases = append(leases, l)
	}
	other, err := Register(WithSession(t.Context(), "example-other", "example-user", func(context.Context) error { return nil }), func() { t.Error("another session closed") })
	require.NoError(t, err)
	defer other.Release()
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() { Revoke("example-session", "", "") })
	}
	workers.Wait()
	for _, l := range leases {
		l.Close()
		l.Release()
	}
	require.EqualValues(t, 2, closed.Load())
}

func TestSessionRegistryRegistersBeforeDurableValidation(t *testing.T) {
	entered, proceed := make(chan struct{}), make(chan struct{})
	closed := make(chan struct{})
	ctx := WithSession(t.Context(), "example-racing", "example-user", func(context.Context) error { close(entered); <-proceed; return errors.New("revoked") })
	done := make(chan error, 1)
	go func() { _, err := Register(ctx, func() { close(closed) }); done <- err }()
	<-entered
	Revoke("example-racing", "", "")
	// Revoke must find the registration while validation is still in flight.
	select {
	case <-closed:
	default:
		t.Error("accept/revoke race left viewer open")
	}
	close(proceed)
	require.Error(t, <-done)
	viewers.Lock()
	remaining := len(viewers.entries)
	viewers.Unlock()
	require.Zero(t, remaining)
}

func TestSessionRegistryRejectsUnreadableStateAndUnboundCredentials(t *testing.T) {
	closed := false
	_, err := Register(WithSession(t.Context(), "example-rejected", "example-user", func(context.Context) error { return errors.New("store unavailable") }), func() { closed = true })
	require.Error(t, err)
	require.True(t, closed)
	l, err := Register(t.Context(), func() {})
	require.NoError(t, err)
	l.Release()
	require.NoError(t, Check(t.Context()))
}
