// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original hub discovery and installation metadata.

package environment

import (
	"context"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
)

var pinnedAgentImage = regexp.MustCompile(`^[a-zA-Z0-9./:_-]+:[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.-]+)?(?:@sha256:[a-f0-9]{64})?$`)
var imageVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.-]+)?$`)

func agentImageReference(cfg *config.Config) (string, error) {
	image := strings.TrimSpace(cfg.AgentImage)
	if image == "" {
		tag := strings.TrimPrefix(config.Version, "v")
		if !imageVersion.MatchString(tag) {
			tag = "0.0.0"
		}
		// The deploy/hub/Dockerfile agent target supplies this registry-free fallback.
		image = "docklet-next-agent:" + tag
	}
	if !pinnedAgentImage.MatchString(image) {
		return "", huma.Error503ServiceUnavailable("Configure a pinned DOCKLET_AGENT_IMAGE")
	}
	return image, nil
}

func hubDiscovery(cfg *config.Config, publicURL, hubAddress string, addresses []string) (environmenttypes.HubDiscovery, error) {
	image, err := agentImageReference(cfg)
	if err != nil {
		return environmenttypes.HubDiscovery{}, err
	}
	result := environmenttypes.HubDiscovery{DetectedAt: time.Now().UTC(), AgentImage: image, Candidates: []environmenttypes.HubAddressCandidate{}}
	seen := map[string]bool{}
	add := func(raw, source string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return
		}
		parsed.Path = ""
		value := parsed.String()
		if seen[value] {
			return
		}
		seen[value] = true
		reach := "unknown"
		if ip := net.ParseIP(parsed.Hostname()); (ip != nil && ip.IsLoopback()) || parsed.Hostname() == "localhost" {
			reach = "same-host"
		}
		result.Candidates = append(result.Candidates, environmenttypes.HubAddressCandidate{ID: source + ":" + value, URL: value, Reach: reach, Source: source})
	}
	add(publicURL, "configured")
	add(cfg.GetAppURL(), "configured")
	add(hubAddress, "configured")
	slices.Sort(addresses)
	for _, address := range addresses {
		ip := net.ParseIP(address)
		if ip == nil || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
			continue
		}
		scheme := "http"
		if cfg.TLSEnabled {
			scheme = "https"
		}
		port := cfg.Port
		if port == "" {
			port = "3552"
		}
		add(scheme+"://"+net.JoinHostPort(address, port), "interface")
	}
	return result, nil
}

func (h *EnvironmentHandler) DiscoverHub(ctx context.Context, _ *struct{}) (*handlerutil.Out[environmenttypes.HubDiscovery], error) {
	settings := h.settingsService.GetSettingsOrDefaults(ctx)
	addresses := []string{}
	interfaces, err := net.InterfaceAddrs()
	if err == nil {
		for _, address := range interfaces {
			ip, _, parseErr := net.ParseCIDR(address.String())
			if parseErr == nil {
				addresses = append(addresses, ip.String())
			}
		}
	}
	discovery, err := hubDiscovery(h.cfg, settings.DomainURL.Value, settings.HubAddress.Value, addresses)
	if err != nil {
		return nil, err
	}
	return &handlerutil.Out[environmenttypes.HubDiscovery]{Body: base.ApiResponse[environmenttypes.HubDiscovery]{Success: true, Data: discovery}}, nil
}
