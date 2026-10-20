// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

package sessionlife

import (
	"context"
	"time"
)

// Cadence keeps transport-owned timing injectable without a global scheduler.
type Cadence struct {
	C     <-chan time.Time
	Stop  func()
	Reset func(time.Duration)
}
type cadenceKey struct{}
type CadenceFactory func(time.Duration, bool) Cadence

func WithCadence(ctx context.Context, factory CadenceFactory) context.Context {
	return context.WithValue(ctx, cadenceKey{}, factory)
}
func NewTicker(ctx context.Context, period time.Duration) Cadence {
	return newCadence(ctx, period, false)
}
func NewTimer(ctx context.Context, period time.Duration) Cadence {
	return newCadence(ctx, period, true)
}
func newCadence(ctx context.Context, period time.Duration, once bool) Cadence {
	if factory, ok := ctx.Value(cadenceKey{}).(CadenceFactory); ok {
		return factory(period, once)
	}
	if once {
		timer := time.NewTimer(period)
		return Cadence{timer.C, func() { timer.Stop() }, func(d time.Duration) { timer.Reset(d) }}
	}
	ticker := time.NewTicker(period)
	return Cadence{ticker.C, ticker.Stop, ticker.Reset}
}
