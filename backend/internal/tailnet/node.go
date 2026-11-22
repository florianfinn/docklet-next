// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original embedded tsnet node boundary for the optional Tailscale route.

// Package tailnet owns the process's single embedded Tailscale node (hub or agent).
package tailnet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"tailscale.com/client/local"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/envknob"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

// IngressPort is the hub's tailnet ingress port; agents reach the hub at http://<hub tailnet IPv4>:3552.
const IngressPort = "3552"

const (
	HubTag   = "tag:docklet-hub"
	AgentTag = "tag:docklet-agent"
)

// watchMask seeds from the initial status and receives state, health and peer-set changes.
const watchMask = ipn.NotifyInitialStatus | ipn.NotifyInitialState | ipn.NotifyInitialHealthState | ipn.NotifyPeerChanges

// NodeConfig configures one embedded node. AuthKey is consumed only while the state has no identity;
// Secrets are exact values redacted from every tsnet log line in addition to the tskey- pattern.
type NodeConfig struct {
	Dir           string
	Hostname      string
	ControlURL    string
	AuthKey       string
	AdvertiseTags []string
	Secrets       []string
}

// HasNodeState reports whether dir holds a logged-in node identity from an earlier join. tsnet creates
// Dir/tailscaled.state before enrolling, so only a stored login profile (a "profile-" key, written once the
// node has a node ID) counts; an initialised but unenrolled store does not.
func HasNodeState(dir string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, "tailscaled.state"))
	if err != nil || len(raw) > 4<<20 {
		return false
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil {
		return false
	}
	for key := range keys {
		if strings.HasPrefix(key, "profile-") {
			return true
		}
	}
	return false
}

// Watcher is one IPN bus subscription.
type Watcher interface {
	Next() (ipn.Notify, error)
	Close() error
}

// Node is the subset of tsnet the route needs; tests substitute fakes.
type Node interface {
	Up(ctx context.Context) error
	Listen(network, address string) (net.Listener, error)
	Dial(ctx context.Context, network, address string) (net.Conn, error)
	Status(ctx context.Context) (*ipnstate.Status, error)
	Watch(ctx context.Context) (Watcher, error)
	WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
	Close() error
}

// NodeFactory creates a node that has not started yet.
type NodeFactory func(NodeConfig) (Node, error)

type tsnetNode struct {
	server *tsnet.Server
}

// NewTSNetNode is the production factory. Log upload is disabled process-wide
// and tsnet logs go to slog at debug level with credentials redacted.
func NewTSNetNode(cfg NodeConfig) (Node, error) {
	envknob.SetNoLogsNoSupport()
	logf := func(format string, args ...any) {
		slog.Debug("tailscale", "message", redactSecrets(fmt.Sprintf(format, args...), cfg.Secrets...))
	}
	return &tsnetNode{server: &tsnet.Server{
		Dir:           cfg.Dir,
		Hostname:      cfg.Hostname,
		ControlURL:    cfg.ControlURL,
		AuthKey:       cfg.AuthKey,
		AdvertiseTags: cfg.AdvertiseTags,
		Logf:          logf,
		UserLogf:      logf,
	}}, nil
}

func (n *tsnetNode) Up(ctx context.Context) error {
	_, err := n.server.Up(ctx)
	return err
}

func (n *tsnetNode) Listen(network, address string) (net.Listener, error) {
	return n.server.Listen(network, address)
}

func (n *tsnetNode) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return n.server.Dial(ctx, network, address)
}

func (n *tsnetNode) localClient() (*local.Client, error) { return n.server.LocalClient() }

func (n *tsnetNode) Status(ctx context.Context) (*ipnstate.Status, error) {
	lc, err := n.localClient()
	if err != nil {
		return nil, err
	}
	return lc.Status(ctx)
}

func (n *tsnetNode) Watch(ctx context.Context) (Watcher, error) {
	lc, err := n.localClient()
	if err != nil {
		return nil, err
	}
	return lc.WatchIPNBus(ctx, watchMask)
}

func (n *tsnetNode) WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
	lc, err := n.localClient()
	if err != nil {
		return nil, err
	}
	return lc.WhoIs(ctx, remoteAddr)
}

func (n *tsnetNode) Close() error { return n.server.Close() }

var credentialPattern = regexp.MustCompile(`tskey-[A-Za-z0-9_-]+(-[A-Za-z0-9_-]+)*(\?[^\s"']*)?`)

// Redact removes Tailscale auth keys and OAuth secrets from log text.
func Redact(text string) string {
	return credentialPattern.ReplaceAllString(text, "tskey-[redacted]")
}

// redactSecrets removes exact secret values as well, for values that lack the tskey prefix.
func redactSecrets(text string, secrets ...string) string {
	text = Redact(text)
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); len(secret) >= 8 {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	return text
}
