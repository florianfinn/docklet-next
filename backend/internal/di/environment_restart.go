// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original application-lifetime wiring for the hub restart controller.

package di

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"go.uber.org/fx"
	"net/http"
)

func provideRestartEnvironmentService(appCtx context.Context, lc fx.Lifecycle, cfg *config.Config, db *database.DB, httpClient *http.Client, dockerService *docker.DockerClientService, eventService *event.EventService, settingsService *settings.SettingsService, apiKeys *apikey.ApiKeyService) *environment.EnvironmentService {
	service := environment.NewEnvironmentService(db, httpClient, dockerService, eventService, settingsService, apiKeys)
	if !cfg.AgentMode {
		lc.Append(fx.Hook{OnStart: func(context.Context) error { return service.StartAgentRestarts(appCtx) }, OnStop: func(context.Context) error { service.StopAgentRestarts(); return nil }})
	}
	return service
}
