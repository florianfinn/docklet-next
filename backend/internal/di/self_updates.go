// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package di

import (
	"context"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"go.uber.org/fx"
)

func wireSelfUpdates(ctx context.Context, lc fx.Lifecycle, system *system.SystemService, environments *environment.EnvironmentService, backups *backup.Engine, activities *activity.ActivityService) {
	updates := system.SelfUpdates()
	updates.SetEnvironments(environments)
	backups.SetHostAdmission(func(ctx context.Context) error { return activities.CheckSelfUpdateHost(ctx, "0") })
	lc.Append(fx.Hook{OnStart: func(context.Context) error { return updates.Start(ctx) }, OnStop: func(context.Context) error { updates.Stop(); return nil }})
}
