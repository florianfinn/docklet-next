// Source: getarcaneapp/arcane/backend/internal/environment/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories; functional cut: removed S3 backup destinations.

package environment

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/version"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// GetDeploymentSnippets returns deployment snippets for an environment.
func (h *EnvironmentHandler) GetDeploymentSnippets(ctx context.Context, input *GetDeploymentSnippetsInput) (*handlerutil.Out[DeploymentSnippet], error) {
	env, err := h.environmentService.GetEnvironmentByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("Environment not found")
	}

	if env.ApiKeyID == nil {
		return nil, huma.Error400BadRequest("Environment does not have an API key configured")
	}

	if env.AccessToken == nil || *env.AccessToken == "" {
		return nil, huma.Error400BadRequest("Environment is missing access token")
	}

	// Generate snippets with API key
	// Use edge snippets for edge environments
	var snippets *DeploymentSnippets
	if env.IsEdge {
		snippets, err = h.environmentService.GenerateEdgeDeploymentSnippets(ctx, env.ID, h.cfg.GetAppURL(), *env.AccessToken, &edge.Config{
			EdgeMTLSMode:      h.cfg.EdgeMTLSMode,
			EdgeMTLSCAFile:    h.cfg.EdgeMTLSCAFile,
			EdgeMTLSAssetsDir: h.cfg.EdgeMTLSAssetsDir,
			AppURL:            h.cfg.GetAppURL(),
		})
	} else {
		snippets, err = h.environmentService.GenerateDeploymentSnippets(ctx, env.ID, h.cfg.GetAppURL(), env.ApiUrl, *env.AccessToken)
	}
	if err != nil {
		slog.ErrorContext(ctx, "Failed to generate deployment snippets", "environmentId", input.ID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to generate deployment snippets")
	}

	var mtls *DeploymentSnippetMTLS
	if snippets.MTLS != nil {
		files := make([]DeploymentSnippetFile, 0, len(snippets.MTLS.Files))
		for _, file := range snippets.MTLS.Files {
			sensitive := isSensitiveMTLSAssetNameInternal(file.Name)
			entry := DeploymentSnippetFile{
				Name:          file.Name,
				ContainerPath: file.ContainerPath,
				Permissions:   file.Permissions,
				DownloadURL:   fmt.Sprintf("/api/environments/%s/deployment/mtls/%s", env.ID, file.Name),
			}
			if sensitive {
				entry.Sensitive = true
			} else {
				entry.Content = file.Content
			}
			files = append(files, entry)
		}
		mtls = &DeploymentSnippetMTLS{
			DockerRun:     snippets.MTLS.DockerRun,
			DockerCompose: snippets.MTLS.DockerCompose,
			Files:         files,
			HostDirHint:   snippets.MTLS.HostDirHint,
		}
	}

	return &handlerutil.Out[DeploymentSnippet]{
		Body: base.ApiResponse[DeploymentSnippet]{
			Success: true,
			Data: DeploymentSnippet{
				DockerRun:     snippets.DockerRun,
				DockerCompose: snippets.DockerCompose,
				MTLS:          mtls,
			},
		},
	}, nil
}

// GetEnvironmentVersion returns the version of a remote environment.
func (h *EnvironmentHandler) GetEnvironmentVersion(ctx context.Context, input *GetEnvironmentVersionInput) (*handlerutil.Out[version.Info], error) {
	env, err := h.environmentService.GetEnvironmentByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("Environment not found")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var versionInfo version.Info

	// For edge environments, route through the tunnel
	if env.IsEdge {
		if !edge.HasActiveTunnel(input.ID) {
			if _, ok := edge.RequestTunnelAndWait(reqCtx, input.ID, edge.DefaultTunnelDemandTTL, edge.DefaultTunnelAcquireTimeout()).Get(); !ok {
				return nil, huma.Error503ServiceUnavailable("Edge agent is not connected")
			}
		}

		statusCode, respBody, doRequestErr := edge.DoRequest(reqCtx, input.ID, http.MethodGet, "/api/app-version", nil)
		if doRequestErr != nil {
			return nil, huma.Error500InternalServerError("Request via tunnel failed: " + doRequestErr.Error())
		}
		if statusCode != http.StatusOK {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("Unexpected status code: %d", statusCode))
		}

		if unmarshalErr := json.Unmarshal(respBody, &versionInfo); unmarshalErr != nil {
			return nil, huma.Error500InternalServerError("Failed to decode version response")
		}
	} else {
		// Direct HTTP request for non-edge environments
		validatedURL, validateErr := httpx.ValidateOutboundHTTPURL(env.ApiUrl)
		if validateErr != nil {
			return nil, huma.Error400BadRequest("Invalid environment API URL")
		}
		validatedURL.RawQuery = ""
		validatedURL.Fragment = ""
		validatedURL.Path = strings.TrimRight(validatedURL.Path, "/") + "/api/app-version"

		req, newRequestWithContextErr := http.NewRequestWithContext(reqCtx, http.MethodGet, validatedURL.String(), http.NoBody)
		if newRequestWithContextErr != nil {
			return nil, huma.Error500InternalServerError("Failed to create request")
		}

		client := &http.Client{Timeout: 15 * time.Second}
		resp, newRequestWithContextErr := client.Do(req)
		if newRequestWithContextErr != nil {
			return nil, huma.Error500InternalServerError("Request failed: " + newRequestWithContextErr.Error())
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("Unexpected status code: %d", resp.StatusCode))
		}

		if unmarshalReadErr := json.UnmarshalRead(resp.Body, &versionInfo); unmarshalReadErr != nil {
			return nil, huma.Error500InternalServerError("Failed to decode version response")
		}
	}

	// Update environment status to online since we successfully contacted it
	if updateErr := h.environmentService.UpdateEnvironmentHeartbeat(ctx, input.ID); updateErr != nil {
		slog.WarnContext(ctx, "Failed to update environment heartbeat", "environmentId", input.ID, "error", updateErr)
		// Don't fail the request if heartbeat update fails
	}

	return &handlerutil.Out[version.Info]{
		Body: base.ApiResponse[version.Info]{
			Success: true,
			Data:    versionInfo,
		},
	}, nil
}
