// Source: getarcaneapp/arcane/backend/internal/environment/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories; functional cut: removed S3 backup destinations.

package environment

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/version"
)

// GetDeploymentSnippets refuses stored-token reads; the browser generates installers.
func (h *EnvironmentHandler) GetDeploymentSnippets(ctx context.Context, input *GetDeploymentSnippetsInput) (*handlerutil.Out[DeploymentSnippet], error) {
	return nil, huma.Error410Gone("Installation credentials are returned only on host creation or token rotation")
}

// GetEnvironmentVersion returns the version of a remote environment.
func (h *EnvironmentHandler) GetEnvironmentVersion(ctx context.Context, input *GetEnvironmentVersionInput) (*handlerutil.Out[version.Info], error) {
	info, err := h.environmentService.CheckAgentCompatibility(ctx, input.ID)
	if err != nil && info == nil {
		return nil, huma.Error503ServiceUnavailable("Agent version is unavailable")
	}
	return &handlerutil.Out[version.Info]{Body: base.ApiResponse[version.Info]{Success: true, Data: *info}}, nil
}
