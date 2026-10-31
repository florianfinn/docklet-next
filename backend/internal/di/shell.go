// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shared hub shell registry wiring for rights revocation.

package di

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/containershell"
)

func provideShellSessions(ctx context.Context, service *activity.ActivityService) *containershell.Registry {
	return containershell.New(service, ctx)
}
