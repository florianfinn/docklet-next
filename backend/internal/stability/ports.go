// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observation ports and real monotonic clock.

package stability

import (
	"context"
	"time"

	activity "github.com/florianfinn/docklet-next/types/activity"
)

const (
	Window        = 120 * time.Second
	Interval      = 2 * time.Second
	Grace         = 5 * time.Second
	// CodeUnstable is listed in the types/image reason vocabulary.
	CodeUnstable  = "update.unstable"
	CodeCancelled = string(activity.CodeRuntimeCancelled)
)

// Clock keeps t0, waits and engine deadlines in the same monotonic time domain.
// WithDeadline must expose its deadline and close Done when it is reached.
type Clock interface {
	Now() time.Time
	WithDeadline(context.Context, time.Time) (context.Context, context.CancelFunc)
	Wait(context.Context, time.Time) error
}

// RealClock preserves time.Time's monotonic component. Use its Now for t0.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }
func (RealClock) WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, deadline)
}
func (RealClock) Wait(ctx context.Context, until time.Time) error {
	timer := time.NewTimer(time.Until(until))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Instance is listed by exact compose project/service labels, including stopped
// containers. OneOff means the com.docker.compose.oneoff label proves true.
type Instance struct {
	ID     string
	OneOff bool
}

// Container is an exact-ID inspect result. Empty or "none" Health means no healthcheck;
// State and Health use Docker's running/exited and starting/healthy/unhealthy values.
type Container struct {
	ID           string
	State        string
	ExitCode     int
	Health       string
	RestartCount int
	StartedAt    time.Time
	OneOff       bool
}

// Engine implementations must honour cancellation/deadlines and join their work
// before returning. List includes stopped/exited replicas and proven one-offs;
// Inspect must use the supplied exact ID, never a name or representative replica.
type Engine interface {
	List(ctx context.Context, project, service string) ([]Instance, error)
	Inspect(ctx context.Context, id string) (Container, error)
}

type Verdict struct {
	Pass      bool
	Cancelled bool
	Code      string
	// Detail is bounded, neutral diagnostic text; engine errors and identities are
	// not copied into it. Callers may retain it as private execution evidence.
	Detail string
}

type Observer struct {
	clock  Clock
	engine Engine
}

func New(clock Clock, engine Engine) *Observer { return &Observer{clock: clock, engine: engine} }
