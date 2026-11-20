// Source: docklet-next/issues/528
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 Traefik setup source eligibility.

package traefik

import (
	"context"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

// SourceEligibility selects only the source holding the file provider directory;
// the route directory is then fixed to that directory, relative to the source.
func (e *engine) SourceEligibility(ctx context.Context, i dto.Instance) (dto.SourceEligibility, error) {
	v, err := e.inventory(ctx, i)
	if err != nil {
		if proxy.Transient(err) {
			return dto.SourceEligibility{}, err
		}
		return dto.SourceEligibility{Reason: reason(err, dto.IncompleteInventory)}, nil
	}
	dir := v.FileProvider
	switch {
	case v.single:
		return dto.SourceEligibility{Reason: dto.SingleFileMount}, nil
	case dir == "":
		return dto.SourceEligibility{Reason: dto.IncompleteInventory}, nil
	case !within(i.Source.MountTarget, dir):
		return dto.SourceEligibility{Reason: dto.ProviderDirectoryElsewhere}, nil
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(dir, strings.TrimSuffix(i.Source.MountTarget, "/")), "/")
	if rel == "" {
		rel = "."
	}
	return dto.SourceEligibility{Selectable: true, RouteDirectory: &dto.RouteDirectoryHint{Value: rel, Fixed: true}}, nil
}
