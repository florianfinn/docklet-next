// Source: docklet-next/issues/519
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original process-local hub container detection with retry on errors and a fallback for custom Docker data roots.

package environment

import (
	"context"
	"os"
	"regexp"
	"sort"
	"strings"
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

// hubContainerID adds a fallback to the shared cgroup lookup for layouts it does not know: a Docker data
// root outside /var/lib/docker, a cgroup v2 path that ends in the ID, and host networking, where the
// hostname is the host's. Inspect still has to confirm the ID, so a wrong match only yields "unknown".
func hubContainerID(primary func() (string, error)) func() (string, error) {
	return func() (string, error) {
		if id, err := primary(); err == nil && id != "" {
			return id, nil
		}
		mountinfo, _ := os.ReadFile("/proc/self/mountinfo")
		cgroups, _ := os.ReadFile("/proc/self/cgroup")
		if id := containerIDFromProcInternal(string(mountinfo), string(cgroups)); id != "" {
			return id, nil
		}
		return "", os.ErrNotExist
	}
}

var (
	containerMountID  = regexp.MustCompile(`/containers/([0-9a-f]{64})/(?:hostname|hosts|resolv\.conf) `)
	containerCgroupID = regexp.MustCompile(`^0::.*/(?:docker-)?([0-9a-f]{64})(?:\.scope)?$`)
)

// containerIDFromProcInternal reads the container ID from the files Docker bind-mounts into every
// container, then from a cgroup v2 path that ends in it.
func containerIDFromProcInternal(mountinfo, cgroups string) string {
	if m := containerMountID.FindStringSubmatch(mountinfo); m != nil {
		return m[1]
	}
	for _, line := range strings.Split(cgroups, "\n") {
		if m := containerCgroupID.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return m[1]
		}
	}
	return ""
}
