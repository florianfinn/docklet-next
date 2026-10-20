// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Actual Fx provider failure must release its application-context registration.

package di

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

type runtimeConstructionContext struct {
	context.Context
	done       chan struct{}
	registered atomic.Int32
	stopped    atomic.Int32
}

func (c *runtimeConstructionContext) Done() <-chan struct{} { return c.done }
func (c *runtimeConstructionContext) AfterFunc(func()) func() bool {
	c.registered.Add(1)
	var stopped atomic.Bool
	return func() bool {
		if !stopped.CompareAndSwap(false, true) {
			return false
		}
		c.stopped.Add(1)
		return true
	}
}

func TestRuntimeActivationConstructionFailureCancelsContext(t *testing.T) {
	parent := &runtimeConstructionContext{Context: context.Background(), done: make(chan struct{})}
	app := fx.New(
		fx.NopLogger,
		fx.Provide(func() context.Context { return parent }),
		fx.Supply(
			activity.NewActivityService(nil, nil),
			(*project.ProjectService)(nil),
			(*container.ContainerService)(nil),
			(*environment.EnvironmentService)(nil),
			(*settings.SettingsService)(nil),
			&config.Config{}, &http.Client{}, edge.NewTunnelRegistry(),
		),
		RuntimeOptions,
		fx.Invoke(func(*activity.RuntimeService) {}),
	)
	require.Error(t, app.Err())
	require.EqualValues(t, 1, parent.registered.Load(), "actual runtime provider must construct the child context")
	require.EqualValues(t, 1, parent.stopped.Load(), "failed construction must release the parent registration")
}
