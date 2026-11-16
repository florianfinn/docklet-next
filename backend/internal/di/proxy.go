// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine registration and journal startup settlement after it.

package di

import (
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/caddy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/traefik"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"go.uber.org/fx"
)

// registerEngines adds every engine before startup settlement can need one.
func registerEngines(m *proxy.Module) {
	m.RegisterEngine(dto.Caddy, caddy.New(m.Runtime))
	m.RegisterEngine(dto.Traefik, traefik.New(m.Runtime))
}

func init() {
	ServiceOptions = fx.Options(ServiceOptions, fx.Invoke(func(lc fx.Lifecycle, m *proxy.Module) {
		registerEngines(m)
		lc.Append(fx.Hook{OnStart: m.Start, OnStop: m.Stop})
	}))
}
