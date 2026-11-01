// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation; M15 simple-user rights and identity contracts.

// Package sessionlife binds viewer lifetimes to durable browser sessions without
// coupling transport packages to authentication or database implementations.
package sessionlife

import (
	"context"
	"sync"
	"time"
)

type binding struct {
	sessionID, userID string
	validate          func(context.Context) error
}
type contextKey struct{}

// WithSession carries only a local revocation key and durable validator. It is
// never serialized or forwarded to an agent.
func WithSession(ctx context.Context, sessionID, userID string, validate func(context.Context) error) context.Context {
	if sessionID == "" && userID == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, &binding{sessionID, userID, validate})
}

var viewers = struct {
	sync.Mutex
	entries map[*Lease]*binding
}{entries: make(map[*Lease]*binding)}

// Lease owns one closer. Close and Release are concurrent and idempotent.
type Lease struct {
	once  sync.Once
	close func()
}

func (l *Lease) Release() {
	if l != nil {
		viewers.Lock()
		delete(viewers.entries, l)
		viewers.Unlock()
	}
}
func (l *Lease) Close() {
	if l != nil {
		l.once.Do(func() { l.Release(); l.close() })
	}
}

// Register publishes the closer before reading durable state, so a revoke
// racing acceptance either sees this handle or is observed by the recheck.
func Register(ctx context.Context, close func()) (*Lease, error) {
	b, _ := ctx.Value(contextKey{}).(*binding)
	if b == nil {
		return &Lease{close: close}, nil
	}
	l := &Lease{close: close}
	viewers.Lock()
	viewers.entries[l] = b
	viewers.Unlock()
	if err := Check(ctx); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Check bypasses authentication caches and bounds each durable store read.
// Cadence belongs to the transport, not to an expiry scheduler.
func Check(ctx context.Context) error {
	b, _ := ctx.Value(contextKey{}).(*binding)
	if b == nil {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return b.validate(checkCtx)
}

// Revoke closes matching handles after the caller's durable commit. Never call
// this from an uncommitted transaction. External closers run outside the lock.
func Revoke(sessionID, userID, exceptSessionID string) {
	var closing []*Lease
	viewers.Lock()
	for l, b := range viewers.entries {
		if (sessionID == "" || b.sessionID == sessionID) && (userID == "" || b.userID == userID) && (exceptSessionID == "" || b.sessionID != exceptSessionID) {
			delete(viewers.entries, l)
			closing = append(closing, l)
		}
	}
	viewers.Unlock()
	for _, l := range closing {
		l.Close()
	}
}
