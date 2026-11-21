// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Uninterpolated project API service projections; M34 #548.

package projects

import (
	"context"
	"github.com/compose-spec/compose-go/v2/loader"
	compose "github.com/compose-spec/compose-go/v2/types"
)

// ProjectServiceProjection returns an isolated definition copy; only image is
// copied from execution. No env-file resolution or AutoInject enters this copy.
func ProjectServiceProjection(ctx context.Context, model *compose.Project, root string, mapper *PathMapper) (*compose.Project, error) {
	input, err := ResolveComposeInputs(ctx, root, model.WorkingDir, false)
	if err != nil {
		return nil, err
	}
	if !input.Recorded() {
		return model, nil
	}
	owned := *input
	owned.Injection = nil
	ctx = WithResolvedInputs(ctx, &owned)
	projection, err := LoadComposeProject(ctx, model.ComposeFiles[0], model.Name, root, false, mapper, nil, func(o *loader.Options) {
		o.SkipInterpolation = true
		o.SkipValidation = true
		o.SkipConsistencyCheck = true
		o.SkipResolveEnvironment = true
	}, false, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	for key, service := range projection.Services {
		service.Image = model.Services[key].Image
		projection.Services[key] = service
	}
	return projection, nil
}
