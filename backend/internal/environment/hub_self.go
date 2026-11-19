// Source: docklet-next/issues/519
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original process-local hub container detection with retry on errors.

package environment

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker/compat"
)

type hubSelf struct {
	inspect container.InspectResponse
	drivers map[string]string
}

type hubSelfDetector struct {
	mu     sync.Mutex
	cached *hubSelf
}

// A hub has one local Docker daemon and one immutable network snapshot per process.
var processHubSelf hubSelfDetector

func (d *hubSelfDetector) detect(ctx context.Context, selfID func() (string, error), getClient func(context.Context) (client.APIClient, error)) *hubSelf {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cached != nil {
		return d.cached
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	id, err := selfID()
	if err != nil || id == "" {
		return nil
	}
	engine, err := getClient(ctx)
	if err != nil {
		return nil
	}
	inspected, err := compat.ContainerInspectWithCompatibility(ctx, engine, id, client.ContainerInspectOptions{})
	if err != nil {
		return nil
	}
	self := &hubSelf{inspect: inspected.Container, drivers: map[string]string{}}
	if self.inspect.NetworkSettings != nil {
		for _, name := range sortedHubNetworkNames(self.inspect) {
			target := name
			if endpoint := self.inspect.NetworkSettings.Networks[name]; endpoint != nil && endpoint.NetworkID != "" {
				target = endpoint.NetworkID
			}
			inspected, err := compat.NetworkInspectWithCompatibility(ctx, engine, target, client.NetworkInspectOptions{})
			if err != nil {
				return nil
			}
			self.drivers[name] = inspected.Network.Driver
		}
	}
	d.cached = self
	return self
}

func sortedHubNetworkNames(inspect container.InspectResponse) []string {
	names := []string{}
	if inspect.NetworkSettings != nil {
		for name := range inspect.NetworkSettings.Networks {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
