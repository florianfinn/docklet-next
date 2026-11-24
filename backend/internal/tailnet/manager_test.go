// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original lifecycle, watcher and status projection tests of the hub tailnet node.

package tailnet_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"

	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

var manual = tailnet.Credential{Mode: tailnettypes.ModeManual, AuthKey: tailnettest.Key("auth", "example-secret")}

func newManager(t *testing.T, fleet *tailnettest.Fleet, mutate ...func(*tailnet.ManagerConfig)) *tailnet.Manager {
	t.Helper()
	cfg := tailnet.ManagerConfig{
		StateDir:        filepath.Join(t.TempDir(), "tailscale"),
		MTLSDisabled:    true,
		Factory:         fleet.Factory,
		RetryMin:        10 * time.Millisecond,
		RetryMax:        20 * time.Millisecond,
		RefreshInterval: 20 * time.Millisecond,
		JoinTimeout:     time.Second,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	m := tailnet.NewManager(cfg)
	t.Cleanup(func() { m.Close(context.Background()) })
	return m
}

func connectionState(m *tailnet.Manager) string { return m.Status().Connection.State }

func TestManagerConnectsAndRecomputesStateAfterRestart(t *testing.T) {
	fleet := tailnettest.NewFleet()
	release := fleet.HoldUps()
	first := newManager(t, fleet)
	first.Apply(t.Context(), true, manual)
	require.Equal(t, "joining", connectionState(first))
	// Hub restarts while joining: nothing of the old process survives, the new one recomputes from the node.
	first.Close(t.Context())
	release()
	fleet2 := tailnettest.NewFleet()
	second := newManager(t, fleet2)
	second.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return connectionState(second) == "connected" }, 2*time.Second, 5*time.Millisecond)
	status := second.Status()
	require.Equal(t, tailnettest.HubIPv4, status.HubAddress)
	require.Equal(t, "example.com", status.TailnetName)
	require.True(t, status.RouteAvailable)
	require.Equal(t, "not-applicable", status.Provisioning.State)
}

func TestManagerReportsRejectedCredentialWithoutRetrying(t *testing.T) {
	fleet := tailnettest.NewFleet()
	fleet.FailNextUps(errors.New("tsnet.Up: backend: invalid key: unable to validate API key"))
	m := newManager(t, fleet)
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return m.Status().Connection.Code == tailnettypes.CodeCredentialRejected }, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, "error", connectionState(m))
	time.Sleep(60 * time.Millisecond)
	require.EqualValues(t, 1, fleet.Created.Load(), "a rejected credential needs operator action, not retries")
	require.Equal(t, tailnettypes.CodeCredentialRejected, m.Status().RouteCode)
}

func TestManagerRetriesUnreachableControlUntilJoined(t *testing.T) {
	fleet := tailnettest.NewFleet()
	fleet.FailNextUps(context.DeadlineExceeded, context.DeadlineExceeded)
	m := newManager(t, fleet)
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
	require.EqualValues(t, 3, fleet.Created.Load())
}

// A join or refresh that finishes after disable, reset or a credential change must not publish its result.
func TestManagerDropsLateResultsOfOlderGenerations(t *testing.T) {
	for _, change := range []string{"disable", "reset", "credential"} {
		t.Run(change, func(t *testing.T) {
			fleet := tailnettest.NewFleet()
			release := fleet.HoldUps()
			m := newManager(t, fleet)
			m.Apply(t.Context(), true, manual)
			require.Equal(t, "joining", connectionState(m))
			fleet2Release := func() {}
			switch change {
			case "disable":
				m.Apply(t.Context(), false, manual)
			case "reset":
				require.NoError(t, m.Reset(t.Context()))
			case "credential":
				fleet2Release = fleet.HoldUps()
				m.Apply(t.Context(), true, tailnet.Credential{Mode: tailnettypes.ModeManual, AuthKey: tailnettest.Key("auth", "example-other")})
			}
			release()
			time.Sleep(80 * time.Millisecond)
			want := "off"
			if change == "credential" {
				want = "joining"
			}
			require.Equal(t, want, connectionState(m), "a stale join must not report connected")
			require.Empty(t, m.Status().HubAddress)
			fleet2Release()
		})
	}
}

// A status refresh already in flight when the operator disables or resets must not report connected afterwards.
func TestManagerDropsLateRefreshesOfOlderGenerations(t *testing.T) {
	for _, change := range []string{"disable", "reset"} {
		t.Run(change, func(t *testing.T) {
			fleet := tailnettest.NewFleet()
			m := newManager(t, fleet)
			m.Apply(t.Context(), true, manual)
			require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
			entered, release := fleet.HoldStatus()
			fleet.Notify(ipn.Notify{PeersChanged: []*tailcfg.Node{{ID: 3}}})
			<-entered
			if change == "disable" {
				m.Apply(t.Context(), false, manual)
			} else {
				require.NoError(t, m.Reset(t.Context()))
			}
			release()
			time.Sleep(80 * time.Millisecond)
			require.Equal(t, "off", connectionState(m), "a stale refresh must not report connected")
			require.Empty(t, m.Status().HubAddress)
		})
	}
}

func TestManagerClosesOldListenerBeforeReplacement(t *testing.T) {
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet)
	m.SetIngressHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return connectionState(m) == "connected" && fleet.Nodes()[0].Addr() != "" }, 2*time.Second, 5*time.Millisecond)
	oldAddr := fleet.Nodes()[0].Addr()
	m.Apply(t.Context(), false, manual)
	_, err := http.Post("http://"+oldAddr+"/api/tunnel/poll", "text/plain", http.NoBody) //nolint:noctx // Test probe against a closed loopback listener.
	require.Error(t, err, "disable must close the ingress listener")
	require.True(t, fleet.Nodes()[0].Closed())
}

func TestWatcherReseedsAfterBusEOFAndRefreshesOnPeerChanges(t *testing.T) {
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet)
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return connectionState(m) == "connected" && fleet.Watches.Load() >= 1 }, 2*time.Second, 5*time.Millisecond)

	// Peer added, then the bus ends: the reconnect seeds from InitialStatus again.
	fleet.SetPeer(ipnstate.PeerStatus{ID: "agent-node", Online: true, Active: true, Relay: "example"})
	fleet.CloseWatchers()
	require.Eventually(t, func() bool { return fleet.Watches.Load() >= 2 }, 2*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return m.HostPeer("agent-node", nil, nil).State == "online" }, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, "relay", m.HostPeer("agent-node", nil, nil).Path)

	// Peer goes offline (patch) and is then removed (peer-set change): each change triggers a refresh.
	seen := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	fleet.SetPeer(ipnstate.PeerStatus{ID: "agent-node", Online: false, LastSeen: seen})
	fleet.Notify(ipn.Notify{PeerChangedPatch: []*tailcfg.PeerChange{{NodeID: 2}}})
	require.Eventually(t, func() bool { return m.HostPeer("agent-node", nil, nil).State == "offline" }, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, seen, *m.HostPeer("agent-node", nil, nil).LastSeen)
	fleet.RemovePeer("agent-node")
	fleet.Notify(ipn.Notify{PeersRemoved: []tailcfg.NodeID{2}})
	require.Eventually(t, func() bool { return m.HostPeer("agent-node", nil, nil).State == "unknown" }, 2*time.Second, 5*time.Millisecond)
}

func TestStatusRefreshIsBoundedPerInterval(t *testing.T) {
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet, func(c *tailnet.ManagerConfig) { c.RefreshInterval = 200 * time.Millisecond })
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return fleet.Refreshes.Load() >= 1 }, 2*time.Second, 5*time.Millisecond)
	before := fleet.Refreshes.Load()
	for range 50 {
		fleet.Notify(ipn.Notify{PeersChanged: []*tailcfg.Node{{ID: 3}}})
	}
	time.Sleep(300 * time.Millisecond)
	require.LessOrEqual(t, fleet.Refreshes.Load()-before, int64(2), "a burst of bus changes must not exceed one refresh per interval")
}

func TestControlUnreachableAfterGraceWhileRunning(t *testing.T) {
	fleet := tailnettest.NewFleet()
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	m := newManager(t, fleet, func(c *tailnet.ManagerConfig) {
		c.Now = func() time.Time { return clock() }
		c.ControlGrace = time.Minute
	})
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
	fleet.SetStatus(func(s *ipnstate.Status) { s.Self.Online = false })
	fleet.Notify(ipn.Notify{SelfChange: &tailcfg.Node{ID: 1}})
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, "connected", connectionState(m), "a short control blip stays connected")
	later := now.Add(2 * time.Minute)
	clock = func() time.Time { return later }
	fleet.Notify(ipn.Notify{SelfChange: &tailcfg.Node{ID: 1}})
	require.Eventually(t, func() bool { return connectionState(m) == "control-unreachable" }, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, tailnettypes.CodeCoordinationUnreachable, m.Status().Connection.Code)
}

func TestMTLSModeRefusesTheRouteWithoutStartingANode(t *testing.T) {
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet, func(c *tailnet.ManagerConfig) { c.MTLSDisabled = false })
	m.Apply(t.Context(), true, manual)
	status := m.Status()
	require.Equal(t, "error", status.Connection.State)
	require.Equal(t, tailnettypes.CodeMTLSUnsupported, status.Connection.Code)
	require.Equal(t, tailnettypes.CodeMTLSUnsupported, status.RouteCode)
	require.False(t, status.RouteAvailable)
	require.Zero(t, fleet.Created.Load())
	var problem *tailnettypes.Problem
	_, _, err := m.ProvisionHostKey(t.Context(), "tskey-auth-host")
	require.ErrorAs(t, err, &problem)
	require.Equal(t, tailnettypes.CodeMTLSUnsupported, problem.Code)
}

// A provider refusal during key minting marks provisioning only; the node, link 1 and existing hosts stay truthful.
func TestProvisioningRejectionKeepsConnectionTruthful(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"test-only"}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(provider.Close)
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet, func(c *tailnet.ManagerConfig) { c.Provisioner = &tailnet.Provisioner{BaseURL: provider.URL} })
	m.Apply(t.Context(), true, tailnet.Credential{Mode: tailnettypes.ModeOAuth, ClientID: "example-client", ClientSecret: tailnettest.Key("client", "example-secret")})
	require.Eventually(t, func() bool {
		s := m.Status()
		return s.Connection.State == "connected" && s.Provisioning.State == "ok"
	}, 2*time.Second, 5*time.Millisecond)

	_, _, err := m.ProvisionHostKey(t.Context(), "")
	var problem *tailnettypes.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, tailnettypes.CodeCredentialRejected, problem.Code)
	status := m.Status()
	require.Equal(t, "rejected", status.Provisioning.State)
	require.Equal(t, http.StatusForbidden, status.Provisioning.ProviderStatus)
	require.Equal(t, "connected", status.Connection.State)
	require.Equal(t, tailnettypes.CodeCredentialRejected, status.RouteCode, "new hosts cannot get keys")
	require.Equal(t, "none", tailnet.FailingLink(m.Connection(), "online", "online"), "no link-1 failure for existing hosts")
}

func TestResetDeletesOnlyLocalStateAndTurnsTheRouteOff(t *testing.T) {
	fleet := tailnettest.NewFleet()
	dir := filepath.Join(t.TempDir(), "tailscale")
	m := newManager(t, fleet, func(c *tailnet.ManagerConfig) { c.StateDir = dir })
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tailscaled.state"), []byte("{}"), 0o600))
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
	require.NoError(t, m.Reset(t.Context()))
	require.NoDirExists(t, dir)
	require.Equal(t, "off", connectionState(m))
	require.Equal(t, "hub", tailnet.FailingLink(m.Connection(), "online", "online"))
}

func TestFailingLinkIsTheFirstFailureAndStandbyIsHealthy(t *testing.T) {
	up := tailnettypes.Connection{State: "connected"}
	for _, tc := range []struct {
		name       string
		hub        tailnettypes.Connection
		peer, host string
		want       string
	}{
		{"hub off", tailnettypes.Connection{State: "off"}, "offline", "offline", "hub"},
		{"hub control unreachable", tailnettypes.Connection{State: "control-unreachable"}, "online", "online", "hub"},
		{"peer offline", up, "offline", "offline", "peer"},
		{"tunnel down", up, "online", "offline", "tunnel"},
		{"tunnel token rejected", up, "online", "token-rejected", "tunnel"},
		{"poll standby healthy", up, "online", "standby", "none"},
		{"peer not observed yet", up, "unknown", "offline", "tunnel"},
		{"all up", up, "online", "online", "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tailnet.FailingLink(tc.hub, tc.peer, tc.host))
		})
	}
}
