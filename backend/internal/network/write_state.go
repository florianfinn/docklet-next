// Source: docklet-next/docs/design/network-management.md
// Revision: 81b69b33c80a454465f54a1f7289c1bc5b5a3381
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package network

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

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
		c, err := inspectWriteContainer(ctx, cli, summary.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
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
	if errdefs.IsConflict(err) {
		return rejection(409, conflictCode, "Network write conflicts with the current Docker state")
	}
	if errdefs.IsInvalidArgument(err) {
		return rejection(400, invalidCode, "Docker rejected the network write")
	}
	return err
}
