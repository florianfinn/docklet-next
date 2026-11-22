// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original network route choice at Edge host creation with one-use tailnet key provisioning.

package environment

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"go.getarcane.app/kit/normalization"

	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/types/environment"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

type tailnetGrant struct {
	key        string
	managerURL string
}

// prepareNetworkRouteInternal validates the route and, for tailnet, provisions the host key before
// any row exists, so a refused or failed provisioning leaves nothing behind.
func (h *EnvironmentHandler) prepareNetworkRouteInternal(ctx context.Context, env *Environment, body environment.Create) (*tailnetGrant, error) {
	if body.Route == nil {
		if body.TailnetAuthKey != nil {
			return nil, huma.Error400BadRequest("A Tailscale key needs the tailnet route")
		}
		return nil, nil
	}
	route := *body.Route
	if !env.IsEdge {
		return nil, huma.Error400BadRequest("A network route applies to Edge hosts")
	}
	env.NetworkRoute = &route
	if route != tailnettypes.RouteTailnet {
		return nil, nil
	}
	tailnetRoute := h.environmentService.tailnetRoute()
	if tailnetRoute == nil {
		return nil, &tailnettypes.Problem{Code: tailnettypes.CodeNotConnected, Status: 409}
	}
	// A refused name must not consume a minted key; the create transaction checks the name again.
	if err := normalization.Normalize(env); err != nil {
		return nil, err
	}
	if err := validateHostName(h.environmentService.db.WithContext(ctx), "", env.Name); err != nil {
		return nil, err
	}
	pasted := ""
	if body.TailnetAuthKey != nil {
		pasted = *body.TailnetAuthKey
	}
	key, managerURL, err := tailnetRoute.ProvisionHostKey(ctx, pasted)
	if errors.Is(err, tailnet.ErrManualKeyRequired) {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err != nil {
		return nil, err
	}
	return &tailnetGrant{key: key, managerURL: managerURL}, nil
}
