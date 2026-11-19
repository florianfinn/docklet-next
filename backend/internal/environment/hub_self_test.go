// Source: docklet-next/issues/519
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original fake Docker inspect failure, retry and successful-cache tests.

package environment

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

type discoveryDocker struct {
	client.APIClient
	self           *hubSelf
	inspectErr     error
	networkErr     error
	inspectCalls   int
	networkCalls   int
	networkTargets []string
	drivers        map[string]string
}

func (d *discoveryDocker) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	d.inspectCalls++
	return client.ContainerInspectResult{Container: d.self.inspect}, d.inspectErr
}

func (d *discoveryDocker) NetworkInspect(_ context.Context, name string, _ client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	d.networkCalls++
	d.networkTargets = append(d.networkTargets, name)
	driver := "bridge"
	if d.drivers != nil {
		driver = d.drivers[name]
	}
	return client.NetworkInspectResult{Network: network.Inspect{Driver: driver}}, d.networkErr
}

func TestHubSelfUnknownDoesNotOfferInterfaces(t *testing.T) {
	for _, tc := range []struct {
		name                                     string
		id                                       string
		idErr, clientErr, inspectErr, networkErr error
	}{
		{name: "no ID"},
		{name: "ID lookup error", idErr: errors.New("not detected")},
		{name: "socket missing", id: "hub-id", clientErr: errors.New("socket unavailable")},
		{name: "inspect error", id: "hub-id", inspectErr: errors.New("inspect unavailable")},
		{name: "network error", id: "hub-id", networkErr: errors.New("network unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &discoveryDocker{self: bridgeHubSelf(), inspectErr: tc.inspectErr, networkErr: tc.networkErr}
			detector := &hubSelfDetector{}
			self := detector.detect(t.Context(), func() (string, error) { return tc.id, tc.idErr }, func(context.Context) (client.APIClient, error) { return engine, tc.clientErr })
			require.Nil(t, self)
			result, err := buildHubDiscovery(&config.Config{}, "https://hub.example.com", "192.0.2.10:3552", self, []string{"192.0.2.200"}, "192.0.2.20:4552")
			require.NoError(t, err)
			require.Equal(t, "unknown", result.ContainerDetection)
			require.Nil(t, result.Container)
			require.Equal(t, []string{"https://hub.example.com", "http://192.0.2.10:3552", "http://192.0.2.20:4552"}, []string{result.Candidates[0].URL, result.Candidates[1].URL, result.Candidates[2].URL})
			require.Len(t, result.Candidates, 3, "unknown detection must never fall back to interfaces")
			require.Nil(t, detector.cached)
		})
	}
}

func TestHubSelfRetriesAndCachesOnlySuccess(t *testing.T) {
	for _, failure := range []string{"ID", "client", "container", "network"} {
		t.Run(failure, func(t *testing.T) {
			detector := &hubSelfDetector{}
			engine := &discoveryDocker{self: bridgeHubSelf()}
			failed := true
			ids, clients := 0, 0
			selfID := func() (string, error) {
				ids++
				if failed && failure == "ID" {
					return "", errors.New("ID unavailable")
				}
				return "hub-id", nil
			}
			getClient := func(context.Context) (client.APIClient, error) {
				clients++
				if failed && failure == "client" {
					return nil, errors.New("socket unavailable")
				}
				return engine, nil
			}
			if failure == "container" {
				engine.inspectErr = errors.New("inspect unavailable")
			}
			if failure == "network" {
				engine.networkErr = errors.New("network unavailable")
			}
			require.Nil(t, detector.detect(t.Context(), selfID, getClient))
			failed, engine.inspectErr, engine.networkErr = false, nil, nil
			self := detector.detect(t.Context(), selfID, getClient)
			require.NotNil(t, self)
			require.Equal(t, "bridge", self.drivers["example-network"])
			require.Equal(t, "net-id", engine.networkTargets[len(engine.networkTargets)-1], "inspect actual network ID rather than infer driver from NetworkMode")
			inspectCalls, networkCalls, idCalls, clientCalls := engine.inspectCalls, engine.networkCalls, ids, clients
			require.Same(t, self, detector.detect(t.Context(), selfID, getClient))
			require.Equal(t, inspectCalls, engine.inspectCalls)
			require.Equal(t, networkCalls, engine.networkCalls)
			require.Equal(t, idCalls, ids)
			require.Equal(t, clientCalls, clients)
		})
	}
}

func TestHubSelfResolvesEveryAttachment(t *testing.T) {
	self := bridgeHubSelf()
	self.inspect.NetworkSettings.Networks["lan"] = &network.EndpointSettings{NetworkID: "lan-id", IPAddress: netip.MustParseAddr("192.0.2.40")}
	engine := &discoveryDocker{self: self, drivers: map[string]string{"net-id": "bridge", "lan-id": "macvlan"}}
	detector := &hubSelfDetector{}
	detected := detector.detect(t.Context(), func() (string, error) { return "hub-id", nil }, func(context.Context) (client.APIClient, error) { return engine, nil })
	require.NotNil(t, detected)
	require.Equal(t, []string{"net-id", "lan-id"}, engine.networkTargets)
	require.Equal(t, map[string]string{"example-network": "bridge", "lan": "macvlan"}, detected.drivers)
	result, err := buildHubDiscovery(&config.Config{}, "", "", detected, nil, "")
	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	require.Equal(t, "http://192.0.2.40:3552", result.Candidates[0].URL)
}

func TestContainerIDFromProcCustomLayouts(t *testing.T) {
	id := strings.Repeat("ab12", 16)
	mount := "3203 3187 0:43 /srv/docker-data/containers/" + id + "/hostname /etc/hostname rw,noatime - ext4 /dev/example rw\n"
	require.Equal(t, id, containerIDFromProcInternal(mount, ""), "a data root outside /var/lib/docker")
	require.Equal(t, id, containerIDFromProcInternal("", "0::/../"+id+"\n"), "a cgroup v2 path ending in the ID")
	require.Equal(t, id, containerIDFromProcInternal("", "0::/system.slice/docker-"+id+".scope\n"))
	require.Empty(t, containerIDFromProcInternal("12 1 0:1 / / rw - overlay overlay rw\n", "0::/user.slice\n"), "a process outside a container")
	require.Empty(t, containerIDFromProcInternal("1 1 0:1 /containers/short/hostname /etc/hostname rw - ext4 x rw\n", ""))
	found, err := hubContainerID(func() (string, error) { return "primary", nil })()
	require.NoError(t, err)
	require.Equal(t, "primary", found, "the shared lookup wins when it finds an ID")
}
