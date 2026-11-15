// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original journal startup settlement after engine registration.

package di

import (
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"go.uber.org/fx"
)

func init() {
	ServiceOptions = fx.Options(ServiceOptions, fx.Invoke(func(lc fx.Lifecycle, m *proxy.Module) { lc.Append(fx.Hook{OnStart: m.Start, OnStop: m.Stop}) }))
}
