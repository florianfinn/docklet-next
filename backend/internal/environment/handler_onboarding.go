// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original hub discovery and installation metadata; operator agent image overrides are free (K-P3, #362); M29 container-aware discovery.

package environment

import (
	"context"
	"errors"
	"net"

	"github.com/danielgtaylor/huma/v2"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
	productVersion "github.com/florianfinn/docklet-next/types/version"
	"github.com/moby/moby/client"
	"go.getarcane.app/sys/cgroup"
)

// agentImageReference accepts any valid operator override (K-P3); only the
// release-stamped default must be a release pin.
func agentImageReference(cfg *config.Config) (string, error) {
	image, operator := config.AgentImageReference(cfg.AgentImage)
	if operator {
		if _, err := reference.ParseNormalizedNamed(image); err != nil {
			return "", huma.Error503ServiceUnavailable("DOCKLET_AGENT_IMAGE is not a valid image reference")
		}
		return image, nil
	}
	if config.DefaultAgentImage != "" && !productVersion.ReleaseReference(image) {
		return "", huma.Error503ServiceUnavailable("The release default agent image is not pinned; set DOCKLET_AGENT_IMAGE")
	}
	return image, nil
}

// HubDiscoveryInput keeps request metadata out of the public input schema.
type HubDiscoveryInput struct {
	host           string
	forwardedHost  string
	forwardedProto string
	remoteAddr     string
}

func (i *HubDiscoveryInput) Resolve(ctx huma.Context) []error {
	i.host, i.forwardedHost, i.forwardedProto, i.remoteAddr = ctx.Host(), ctx.Header("X-Forwarded-Host"), ctx.Header("X-Forwarded-Proto"), ctx.RemoteAddr()
	return nil
}

func (h *EnvironmentHandler) DiscoverHub(ctx context.Context, input *HubDiscoveryInput) (*handlerutil.Out[environmenttypes.HubDiscovery], error) {
	settings := h.settingsService.GetSettingsOrDefaults(ctx)
	self := processHubSelf.detect(ctx, hubContainerID(cgroup.CurrentContainerID), func(ctx context.Context) (client.APIClient, error) {
		if h.environmentService == nil || h.environmentService.dockerService == nil {
			return nil, errors.New("Docker client unavailable")
		}
		return h.environmentService.dockerService.GetClient(ctx)
	})
	addresses := []string{}
	if interfaces, err := net.InterfaceAddrs(); err == nil {
		for _, address := range interfaces {
			if ip, _, err := net.ParseCIDR(address.String()); err == nil {
				addresses = append(addresses, ip.String())
			}
		}
	}
	host := discoveryRequestHost(h.cfg.TrustedProxies, input.host, input.forwardedHost, input.forwardedProto, input.remoteAddr)
	discovery, err := buildHubDiscovery(h.cfg, settings.DomainURL.Value, settings.HubAddress.Value, self, addresses, host)
	if err != nil {
		return nil, err
	}
	return &handlerutil.Out[environmenttypes.HubDiscovery]{Body: base.ApiResponse[environmenttypes.HubDiscovery]{Success: true, Data: discovery}}, nil
}
