// Source: docklet-next/docs/design/network-management.md
// Revision: 81b69b33c80a454465f54a1f7289c1bc5b5a3381
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract; classify recognized concurrent connect conflicts.

package network

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/danielgtaylor/huma/v2"
	networktypes "github.com/florianfinn/docklet-next/types/network"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker/compat"
)

func writeHTTPError(err error, detail string) error {
	var problem *networktypes.Error
	if errors.As(err, &problem) {
		return problem
	}
	return huma.Error500InternalServerError(detail)
}

func inspectWriteNetwork(ctx context.Context, cli *client.Client, id string) (network.Inspect, error) {
	if strings.TrimSpace(id) == "" {
		return network.Inspect{}, invalidNetwork("Network ID must be nonempty")
	}
	result, err := compat.NetworkInspectWithCompatibility(ctx, cli, id, client.NetworkInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return network.Inspect{}, rejection(404, "not-found", "Network not found")
		}
		return network.Inspect{}, fmt.Errorf("inspect network: %w", err)
	}
	return result.Network, nil
}

func inspectWriteContainer(ctx context.Context, cli *client.Client, id string) (container.InspectResponse, error) {
	if strings.TrimSpace(id) == "" {
		return container.InspectResponse{}, invalidNetwork("Container ID must be nonempty")
	}
	result, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return container.InspectResponse{}, rejection(404, "not-found", "Container not found")
		}
		return container.InspectResponse{}, fmt.Errorf("inspect container: %w", err)
	}
	return result.Container, nil
}

// Use unfiltered daemon inventory for protection, including hidden system and
// stopped containers. Inspect preserves stopped static IPAM reservations.
func inspectAllWriteContainers(ctx context.Context, cli *client.Client) ([]container.InspectResponse, error) {
	list, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list container references: %w", err)
	}
	result := make([]container.InspectResponse, 0, len(list.Items))
	for _, summary := range list.Items {
		inspected, err := cli.ContainerInspect(ctx, summary.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect container reference: %w", err)
		}
		result = append(result, inspected.Container)
	}
	return result, nil
}

// Parse the wire strings explicitly so malformed values fail with a stable code
// before any Docker operation. Preserve IPAM order and omitted IPv4 defaults.
func createOptions(req networktypes.CreateOptions) (client.NetworkCreateOptions, error) {
	opts := client.NetworkCreateOptions{Driver: req.Driver, Options: req.Options, Labels: req.Labels, Internal: req.Internal, EnableIPv4: req.EnableIPv4, EnableIPv6: new(req.EnableIPv6)}
	if req.IPAM == nil {
		return opts, nil
	}
	opts.IPAM = &network.IPAM{Driver: req.IPAM.Driver, Options: req.IPAM.Options}
	for _, raw := range req.IPAM.Config {
		var pool network.IPAMConfig
		var err error
		if raw.Subnet != "" {
			pool.Subnet, err = netip.ParsePrefix(raw.Subnet)
			if err != nil {
				return opts, invalidNetwork("Invalid IPAM subnet")
			}
		}
		if raw.Gateway != "" {
			pool.Gateway, err = netip.ParseAddr(raw.Gateway)
			if err != nil {
				return opts, invalidNetwork("Invalid IPAM gateway")
			}
		}
		if raw.IPRange != "" {
			pool.IPRange, err = netip.ParsePrefix(raw.IPRange)
			if err != nil {
				return opts, invalidNetwork("Invalid IPAM range")
			}
		}
		pool.AuxAddress = make(map[string]netip.Addr, len(raw.AuxAddress))
		for name, value := range raw.AuxAddress {
			addr, err := netip.ParseAddr(value)
			if err != nil {
				return opts, invalidNetwork("Invalid IPAM auxiliary address")
			}
			pool.AuxAddress[name] = addr
		}
		opts.IPAM.Config = append(opts.IPAM.Config, pool)
	}
	return opts, nil
}

func daemonWriteError(err error, conflictCode, invalidCode string) error {
	if errdefs.IsNotFound(err) {
		return rejection(404, "not-found", "Network or container not found")
	}
	// Docker shares 403/409 across endpoint and address conflicts. Only recognized
	// daemon conditions receive a specific code; unknown messages stay generic.
	if conflictCode == "invalid-ip" && (errdefs.IsConflict(err) || errdefs.IsPermissionDenied(err)) {
		message := strings.ToLower(err.Error())
		if (strings.Contains(message, "endpoint with name ") && strings.Contains(message, " already exists in network ")) || strings.Contains(message, " is already attached to network ") {
			return rejection(409, "already-connected", "Container is already connected to this network")
		}
		if strings.Contains(message, "address already in use") || strings.Contains(message, "address already allocated") {
			return rejection(400, "invalid-ip", "Address conflicts with the current Docker state")
		}
		return err
	}

	conflict := errdefs.IsConflict(err) || (errdefs.IsPermissionDenied(err) && (conflictCode == "in-use" || conflictCode == "invalid-ip"))
	if conflict {
		status := 409
		if conflictCode == "invalid-ip" {
			status = 400
		}
		return rejection(status, conflictCode, "Network write conflicts with the current Docker state")
	}
	if errdefs.IsInvalidArgument(err) {
		return rejection(400, invalidCode, "Docker rejected the network write")
	}
	return err
}
