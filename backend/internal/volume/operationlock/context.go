// Source: docklet-next/issues/193
// Revision: fdbb8c1fae620d9f97a485983cdaabb3fa53bca2
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original operation lock lifetime for detached helper creation.

// Package operationlock retains an already acquired volume lock across detached work.
package operationlock

import (
	"context"
	"sync"
)

type contextKey struct{}
type lease struct {
	mu     sync.Mutex
	volume string
	refs   int
	unlock func()
}

// WithContext owns unlock and returns the caller's release. Retained work keeps
// the original read or write lock alive without recursively acquiring RWMutex.
func WithContext(ctx context.Context, volume string, unlock func()) (context.Context, func()) {
	l := &lease{volume: volume, refs: 1, unlock: unlock}
	return context.WithValue(ctx, contextKey{}, l), l.release()
}

func (l *lease) release() func() {
	return sync.OnceFunc(func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.refs--
		if l.refs == 0 {
			l.unlock()
		}
	})
}

// Retain must run while the operation still owns its lock. Every caller releases
// its own reference when its shared helper result settles, even after cancellation.
func Retain(ctx context.Context, volume string) func() {
	l, ok := ctx.Value(contextKey{}).(*lease)
	if !ok || l.volume != volume {
		return func() {}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.refs == 0 {
		panic("cannot retain a released volume operation lock")
	}
	l.refs++
	return l.release()
}
