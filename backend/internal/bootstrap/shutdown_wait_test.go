// Source: docklet-next/issues/245
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original shutdown-wait tests for actor-host loss.

package bootstrap

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/di"
)

func waitResult(t *testing.T, run func() error) error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- run() }()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("waitForShutdown did not return after the shutdown condition")
		return nil
	}
}

func TestWaitForShutdownReturnsActorHostLoss(t *testing.T) {
	appCtx, cancelCause := context.WithCancelCause(context.Background())
	done := make(chan os.Signal)
	cancelCause(errors.Join(di.ErrActorHostLost, errors.New("host is not registered")))
	err := waitResult(t, func() error { return waitForShutdown(context.Background(), appCtx, done) })
	if !errors.Is(err, di.ErrActorHostLost) {
		t.Fatalf("waitForShutdown = %v, want ErrActorHostLost", err)
	}
}

func TestWaitForShutdownSignalIsClean(t *testing.T) {
	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan os.Signal, 1)
	done <- syscall.SIGTERM
	if err := waitResult(t, func() error { return waitForShutdown(context.Background(), appCtx, done) }); err != nil {
		t.Fatalf("signal shutdown = %v, want nil", err)
	}
}

func TestWaitForShutdownParentCancelIsClean(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	appCtx, cancelApp := context.WithCancelCause(parent)
	defer cancelApp(nil)
	cancelParent()
	if err := waitResult(t, func() error { return waitForShutdown(parent, appCtx, make(chan os.Signal)) }); err != nil {
		t.Fatalf("parent cancel = %v, want nil", err)
	}
}

func TestWaitForShutdownOrdinaryAppCancelIsClean(t *testing.T) {
	appCtx, cancelCause := context.WithCancelCause(context.Background())
	cancelCause(nil)
	if err := waitResult(t, func() error { return waitForShutdown(context.Background(), appCtx, make(chan os.Signal)) }); err != nil {
		t.Fatalf("ordinary app cancel = %v, want nil", err)
	}
}

// The production failure hook must reach waitForShutdown as a fatal, typed cause.
func TestActorHostFailureEndsAppWithFatalCause(t *testing.T) {
	appCtx, cancelCause := context.WithCancelCause(context.Background())
	original := errors.New("host is not registered")
	actorHostFailure(cancelCause)(original)
	err := waitResult(t, func() error { return waitForShutdown(context.Background(), appCtx, make(chan os.Signal)) })
	if !errors.Is(err, di.ErrActorHostLost) || !errors.Is(err, original) {
		t.Fatalf("waitForShutdown = %v, want ErrActorHostLost wrapping the host error", err)
	}
}
