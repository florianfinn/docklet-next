// Source: docklet-next/docs/design/network-management.md
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package network

import (
	"context"
	"errors"
	"fmt"

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
