// Source: docklet-next/issues/528
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 Caddy setup source eligibility (C-DR10).

package caddy

import (
	"context"
	"errors"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

// DefaultRouteDirectory is the suggested, editable route directory below the source.
const DefaultRouteDirectory = "docklet"

// SourceEligibility suggests the root Caddyfile only from an explicit --config
// path inside the source; without one the source is ineligible (no inference).
func (e *engine) SourceEligibility(ctx context.Context, i dto.Instance) (dto.SourceEligibility, error) {
	c, err := e.runtime.InspectContainer(ctx, i)
	if err != nil {
		return dto.SourceEligibility{}, err
	}
	root, _, err := command(c)
	if err != nil {
		var p *dto.Problem
		if errors.As(err, &p) && p.Code != dto.IncompleteInventory {
			return dto.SourceEligibility{Reason: p.Code}, nil
		}
		if !errors.As(err, &p) && proxy.Transient(err) {
			return dto.SourceEligibility{}, err
		}
		return dto.SourceEligibility{Reason: dto.CaddyConfigImplicit}, nil
	}
	mount := strings.TrimSuffix(i.Source.MountTarget, "/")
	if !strings.HasPrefix(root, mount+"/") {
		return dto.SourceEligibility{Reason: dto.CaddyConfigImplicit}, nil
	}
	return dto.SourceEligibility{Selectable: true, RootFile: root, RouteDirectory: &dto.RouteDirectoryHint{Value: DefaultRouteDirectory}}, nil
}
