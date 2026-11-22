// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original review-fix proofs (M36-S4A-R01–R07): redaction, retained identity, provisioning order and health.

package tailnet_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tailscale.com/health"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsconst"

	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

var oauth = tailnet.Credential{Mode: tailnettypes.ModeOAuth, ClientID: "example-client", ClientSecret: tailnettest.Key("client", "example-secret")}

// syncBuffer is a log sink safe for concurrent handler writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return buf
}

func retainedState(t *testing.T) func(*tailnet.ManagerConfig) {
	return func(c *tailnet.ManagerConfig) {
		require.NoError(t, os.MkdirAll(c.StateDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(c.StateDir, "tailscaled.state"), []byte(`{"_machinekey":"ZXhhbXBsZQ==","profile-a1b2":"ZXhhbXBsZQ=="}`), 0o600))
	}
}

// F1: a manual key without the tskey- prefix that a control error echoes never reaches the log.
func TestManualCredentialIsRedactedFromJoinLogs(t *testing.T) {
	logs := captureLogs(t)
	secret := strings.Repeat("manual-example-", 3)
	fleet := tailnettest.NewFleet()
	fleet.FailNextUps(errors.New("control said no for "+secret), errors.New("tsnet.Up: backend: invalid key "+secret))
	m := newManager(t, fleet)
	m.Apply(t.Context(), true, tailnet.Credential{Mode: tailnettypes.ModeManual, AuthKey: secret})
	require.Eventually(t, func() bool { return m.Status().Connection.Code == tailnettypes.CodeCredentialRejected }, 2*time.Second, 5*time.Millisecond)
	require.Contains(t, logs.String(), "Tailnet credential rejected")
	require.NotContains(t, logs.String(), secret)
}

// F2: a retained identity starts without the credential, so a failing OAuth exchange cannot stop it.
func TestRetainedNodeStartsWithoutTheCredential(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	t.Cleanup(provider.Close)
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet, retainedState(t), func(c *tailnet.ManagerConfig) { c.Provisioner = &tailnet.Provisioner{BaseURL: provider.URL} })
	m.Apply(t.Context(), true, oauth)
	require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
	require.Empty(t, fleet.Nodes()[0].Config.AuthKey, "a retained node needs no enrollment key")
	require.Eventually(t, func() bool { return m.Status().Provisioning.State == "rejected" }, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, "connected", connectionState(m), "a provisioning refusal keeps the connection")
}

// F2: replacing the credential on a connected node keeps the node, listener and watcher.
func TestCredentialReplacementKeepsTheConnectedNode(t *testing.T) {
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet)
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
	m.Apply(t.Context(), true, tailnet.Credential{Mode: tailnettypes.ModeManual, AuthKey: tailnettest.Key("auth", "example-replacement")})
	time.Sleep(60 * time.Millisecond)
	require.EqualValues(t, 1, fleet.Created.Load(), "no new node for a credential replacement")
	require.False(t, fleet.Nodes()[0].Closed())
	require.Equal(t, "connected", connectionState(m))
}

// F2: a retained identity that waits for a new login enrolls with the credential on the next attempt.
func TestRetainedNodeNeedingLoginEnrolls(t *testing.T) {
	fleet := tailnettest.NewFleet()
	fleet.SetStatus(func(s *ipnstate.Status) { s.BackendState = ipn.NeedsLogin.String() })
	fleet.FailNextUps(context.DeadlineExceeded)
	m := newManager(t, fleet, retainedState(t))
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return fleet.Created.Load() >= 2 }, 2*time.Second, 5*time.Millisecond)
	nodes := fleet.Nodes()
	require.Empty(t, nodes[0].Config.AuthKey)
	require.Equal(t, manual.AuthKey, nodes[1].Config.AuthKey)
}

// F3: a provider failure without a refusal ends checking as unavailable and is retried until accepted.
func TestProviderFailureEndsCheckingAndRetries(t *testing.T) {
	var mu sync.Mutex
	failing := true
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if failing {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"test-only"}`))
	}))
	t.Cleanup(provider.Close)
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet, func(c *tailnet.ManagerConfig) {
		c.Provisioner = &tailnet.Provisioner{BaseURL: provider.URL}
		c.ProvisionRetryMin, c.ProvisionRetryMax = 20*time.Millisecond, 40*time.Millisecond
	})
	m.Apply(t.Context(), true, oauth)
	require.Eventually(t, func() bool { return m.Status().Provisioning.State == "unavailable" }, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, tailnettypes.CodeProviderUnavailable, m.Status().Provisioning.Code)
	require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
	mu.Lock()
	failing = false
	mu.Unlock()
	require.Eventually(t, func() bool { return m.Status().Provisioning.State == "ok" }, 2*time.Second, 5*time.Millisecond)
}

// F4: an older credential check finishing after a newer mint refusal cannot clear that refusal.
func TestOlderCredentialCheckCannotClearMintRefusal(t *testing.T) {
	tokenEntered, releaseToken := make(chan struct{}), make(chan struct{})
	var once sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/oauth/token":
			if r.Header.Get("X-Example-Mint") == "" {
				first := false
				once.Do(func() { first = true })
				if first {
					close(tokenEntered)
					<-releaseToken
				}
			}
			_, _ = w.Write([]byte(`{"access_token":"test-only"}`))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	t.Cleanup(provider.Close)
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet, func(c *tailnet.ManagerConfig) { c.Provisioner = &tailnet.Provisioner{BaseURL: provider.URL} })
	m.Apply(t.Context(), true, oauth)
	<-tokenEntered
	require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
	_, _, err := m.ProvisionHostKey(t.Context(), "")
	var problem *tailnettypes.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "rejected", m.Status().Provisioning.State)
	close(releaseToken)
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, "rejected", m.Status().Provisioning.State, "the older token success must not clear the newer refusal")
	require.Equal(t, tailnettypes.CodeCredentialRejected, m.Status().RouteCode)
}

// F5: the coordination warnables mark control-unreachable while Running and Online; other warnings do not.
func TestCoordinationHealthWarningIsProjected(t *testing.T) {
	fleet := tailnettest.NewFleet()
	m := newManager(t, fleet)
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
	fleet.Notify(ipn.Notify{Health: &health.State{Warnings: map[health.WarnableCode]health.UnhealthyState{"update-available": {}}}})
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, "connected", connectionState(m), "unrelated warnings keep the connection")
	fleet.Notify(ipn.Notify{Health: &health.State{Warnings: map[health.WarnableCode]health.UnhealthyState{tsconst.HealthWarnableMapResponseTimeout: {}}}})
	require.Eventually(t, func() bool { return connectionState(m) == "control-unreachable" }, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, tailnettypes.CodeCoordinationUnreachable, m.Status().Connection.Code)
	fleet.Notify(ipn.Notify{Health: &health.State{}})
	require.Eventually(t, func() bool { return connectionState(m) == "connected" }, 2*time.Second, 5*time.Millisecond)
}

// F6: a key minted while the route is disabled, reset or re-keyed is discarded instead of returned.
func TestStaleMintIsDiscarded(t *testing.T) {
	for _, change := range []string{"disable", "reset", "credential"} {
		t.Run(change, func(t *testing.T) {
			mintEntered, releaseMint := make(chan struct{}), make(chan struct{})
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/oauth/token" {
					_, _ = w.Write([]byte(`{"access_token":"test-only"}`))
					return
				}
				close(mintEntered)
				<-releaseMint
				_, _ = w.Write([]byte(`{"id":"k1","key":"tskey-auth-example-host"}`))
			}))
			t.Cleanup(provider.Close)
			fleet := tailnettest.NewFleet()
			m := newManager(t, fleet, func(c *tailnet.ManagerConfig) { c.Provisioner = &tailnet.Provisioner{BaseURL: provider.URL} })
			m.Apply(t.Context(), true, oauth)
			require.Eventually(t, func() bool { s := m.Status(); return s.Connection.State == "connected" && s.Provisioning.State == "ok" }, 2*time.Second, 5*time.Millisecond)
			type result struct {
				key string
				err error
			}
			done := make(chan result, 1)
			go func() { key, _, err := m.ProvisionHostKey(context.Background(), ""); done <- result{key, err} }()
			<-mintEntered
			switch change {
			case "disable":
				m.Apply(t.Context(), false, oauth)
			case "reset":
				require.NoError(t, m.Reset(t.Context()))
			case "credential":
				m.Apply(t.Context(), true, tailnet.Credential{Mode: tailnettypes.ModeOAuth, ClientID: "example-client", ClientSecret: tailnettest.Key("client", "example-replacement")})
			}
			close(releaseMint)
			got := <-done
			require.Empty(t, got.key, "a stale key is never returned")
			var problem *tailnettypes.Problem
			require.ErrorAs(t, got.err, &problem)
			require.Equal(t, tailnettypes.CodeNotConnected, problem.Code)
		})
	}
}

// F7: the agent route stays on across a keyless restart and fails closed when the identity is lost.
func TestAgentRouteSurvivesKeylessRestart(t *testing.T) {
	dir := t.TempDir()
	require.False(t, tailnet.NewAgentRoute(tailnet.AgentConfig{StateDir: dir, Hostname: "docklet-agent"}).Enabled(), "a default hostname alone is no opt-in")
	require.True(t, tailnet.NewAgentRoute(tailnet.AgentConfig{StateDir: dir, Requested: true}).Enabled(), "the installer's explicit hostname keeps the route on")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tailscaled.state"), []byte(`{}`), 0o600))
	require.False(t, tailnet.HasNodeState(dir), "an initialised but unenrolled store is no identity")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tailscaled.state"), []byte(`{"_machinekey":"ZXhhbXBsZQ==","profile-a1b2":"ZXhhbXBsZQ=="}`), 0o600))
	route := tailnet.NewAgentRoute(tailnet.AgentConfig{StateDir: dir, Factory: tailnettest.NewFleet().Factory})
	require.True(t, route.Enabled(), "a retained identity keeps the route on without the consumed key")
	_, err := route.Dial(t.Context(), "tcp", "192.0.2.1:3552")
	require.Error(t, err, "before joining the route fails closed instead of using normal networking")
	require.NoError(t, route.Join(t.Context()))
	t.Cleanup(func() { _ = route.Close() })
}

// R09: an interrupted first join (tsnet already created an empty store) keeps offering the key.
func TestInterruptedAgentEnrollmentKeepsItsKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tailscaled.state"), []byte(`{}`), 0o600))
	fleet := tailnettest.NewFleet()
	fleet.FailNextUps(context.DeadlineExceeded)
	key := tailnettest.Key("auth", "example-agent")
	agent, err := tailnet.JoinAgent(t.Context(), tailnet.AgentConfig{AuthKey: key, StateDir: dir, Factory: fleet.Factory, RetryMin: 5 * time.Millisecond, RetryMax: 20 * time.Millisecond})
	require.NoError(t, err)
	t.Cleanup(func() { _ = agent.Close() })
	nodes := fleet.Nodes()
	require.Len(t, nodes, 2)
	require.Equal(t, key, nodes[0].Config.AuthKey)
	require.Equal(t, key, nodes[1].Config.AuthKey, "the retry after a failed first join still enrolls")
}
