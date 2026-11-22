// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original wiring of the hub tailnet route into hosts, stream and lifecycle.

package di

import (
	"context"

	"go.uber.org/fx"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
)

func provideTailnetServiceInternal(cfg *config.Config, s *settings.SettingsService) *tailnet.Service {
	return tailnet.NewService(cfg, s)
}

// wireTailnet binds the hub route to the host projection; agents have no service and skip this.
func wireTailnet(lc fx.Lifecycle, appCtx context.Context, service *tailnet.Service, hosts *environment.EnvironmentService) {
	if service == nil || hosts == nil {
		return
	}
	hosts.SetTailnet(service.Manager())
	service.SetHostCounter(hosts.CountTailnetHosts)
	unsubscribe := service.Manager().Subscribe(hosts.NotifyRuntimeStateChanged)
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			service.Start(context.WithoutCancel(appCtx))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			unsubscribe()
			service.Stop(ctx)
			return nil
		},
	})
}

func init() {
	ServiceOptions = fx.Options(ServiceOptions,
		fx.Provide(provideTailnetServiceInternal, tailnet.NewModule),
		fx.Invoke(wireTailnet),
	)
}
