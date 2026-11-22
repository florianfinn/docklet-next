// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original redacted status projection, host peer view and offline chain of the tailnet route.

package tailnet

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"

	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

// ErrManualKeyRequired refuses a manual-mode tailnet onboarding without a pasted host key.
var ErrManualKeyRequired = errors.New("a one-use Tailscale key for the host is required in manual mode")

// Status returns the redacted hub status; host counts are filled by the caller that owns the host list.
func (m *Manager) Status() tailnettypes.Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := tailnettypes.Status{
		Enabled:      m.enabled,
		Connection:   m.conn,
		Provisioning: m.prov,
		Generation:   m.gen,
	}
	if m.enabled {
		out.Mode = m.cred.Mode
	}
	if s := m.status; s != nil && m.conn.State != "off" {
		if s.CurrentTailnet != nil {
			out.TailnetName = s.CurrentTailnet.Name
		}
		out.HubAddress = hubIPv4(s)
	}
	out.RouteCode = m.routeRefusalLocked()
	out.RouteAvailable = out.RouteCode == ""
	return out
}

func hubIPv4(s *ipnstate.Status) string {
	for _, addr := range s.TailscaleIPs {
		if addr.Is4() {
			return addr.String()
		}
	}
	return ""
}

// routeRefusalLocked is the reason new hosts cannot use the tailnet route, or "".
func (m *Manager) routeRefusalLocked() string {
	switch {
	case !m.cfg.MTLSDisabled:
		return tailnettypes.CodeMTLSUnsupported
	case !m.enabled || m.conn.State != "connected" || m.status == nil || hubIPv4(m.status) == "":
		return tailnettypes.CodeNotConnected
	case m.cred.Mode == tailnettypes.ModeOAuth && m.prov.State == "rejected":
		return tailnettypes.CodeCredentialRejected
	}
	return ""
}

// RouteRefusal returns a typed problem when new hosts cannot use the tailnet route.
func (m *Manager) RouteRefusal() error {
	m.mu.Lock()
	code := m.routeRefusalLocked()
	m.mu.Unlock()
	if code == "" {
		return nil
	}
	return &tailnettypes.Problem{Code: code, Status: http.StatusConflict}
}

// ProvisionHostKey returns the one-use key for a new host and the hub URL its installer uses.
// OAuth mode mints a key; manual mode returns the pasted key unchanged. Nothing is stored.
func (m *Manager) ProvisionHostKey(ctx context.Context, pasted string) (string, string, error) {
	m.mu.Lock()
	code := m.routeRefusalLocked()
	gen, cred := m.gen, m.cred
	var managerURL string
	if m.status != nil {
		managerURL = "http://" + hubIPv4(m.status) + ":" + IngressPort
	}
	m.mu.Unlock()
	if code != "" {
		return "", "", &tailnettypes.Problem{Code: code, Status: http.StatusConflict}
	}
	if cred.Mode != tailnettypes.ModeOAuth {
		key := strings.TrimSpace(pasted)
		if key == "" || strings.ContainsAny(key, " \t\r\n") {
			return "", "", ErrManualKeyRequired
		}
		return key, managerURL, nil
	}
	seq := m.beginProvisioning()
	mintCtx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	key, err := m.cfg.Provisioner.MintHostKey(mintCtx, cred.ClientID, cred.ClientSecret)
	m.recordProvisioning(gen, seq, cred, err, false)
	if err != nil {
		return "", "", err
	}
	// A key minted for a disabled, reset or replaced identity is discarded; it expires unused upstream.
	m.mu.Lock()
	stale := m.gen != gen || m.cred != cred || m.routeRefusalLocked() != ""
	m.mu.Unlock()
	if stale {
		return "", "", &tailnettypes.Problem{Code: tailnettypes.CodeNotConnected, Status: http.StatusConflict}
	}
	return key, managerURL, nil
}

// WhoIs resolves a tailnet peer address to its stable node ID and whether it carries the agent tag.
func (m *Manager) WhoIs(ctx context.Context, remoteAddr string) (string, bool, error) {
	m.mu.Lock()
	var node Node
	if m.run != nil {
		node = m.run.node
	}
	m.mu.Unlock()
	if node == nil {
		return "", false, errors.New("tailnet node is not running")
	}
	who, err := node.WhoIs(ctx, remoteAddr)
	if err != nil {
		return "", false, err
	}
	if who == nil || who.Node == nil || who.Node.StableID == "" {
		return "", false, errors.New("tailnet peer has no node identity")
	}
	return string(who.Node.StableID), slices.Contains(who.Node.Tags, AgentTag), nil
}

// Connection returns the hub node's current connection; link 1 uses only this.
func (m *Manager) Connection() tailnettypes.Connection {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conn
}

// HostPeer projects the observed node of one host from the latest status snapshot.
func (m *Manager) HostPeer(nodeID string, tagged *bool, observedAt *time.Time) tailnettypes.HostPeer {
	m.mu.Lock()
	defer m.mu.Unlock()
	peer := tailnettypes.HostPeer{State: "unknown", Path: "unknown", Tagged: tagged, ObservedAt: observedAt}
	if m.status == nil || nodeID == "" || m.conn.State == "off" {
		return peer
	}
	for _, p := range m.status.Peer {
		if p == nil || p.ID != tailcfg.StableNodeID(nodeID) {
			continue
		}
		peer.State = "offline"
		if p.Online {
			peer.State = "online"
		}
		if !p.Online && !p.LastSeen.IsZero() {
			seen := p.LastSeen
			peer.LastSeen = &seen
		}
		peer.Path = peerPath(p)
		return peer
	}
	return peer
}

func peerPath(p *ipnstate.PeerStatus) string {
	switch {
	case !p.Online:
		return "unknown"
	case !p.Active:
		return "idle"
	case p.CurAddr != "":
		return "direct"
	case p.Relay != "" || p.PeerRelay != "":
		return "relay"
	}
	return "unknown"
}

// FailingLink returns the first failing link of hub → peer → tunnel. An unknown peer
// is not a failure (no observation yet), and poll standby is a healthy tunnel state.
func FailingLink(hub tailnettypes.Connection, peerState, hostStatus string) string {
	switch {
	case hub.State != "connected":
		return "hub"
	case peerState == "offline":
		return "peer"
	case hostStatus == "offline" || hostStatus == "error" || hostStatus == "token-rejected":
		return "tunnel"
	}
	return "none"
}
