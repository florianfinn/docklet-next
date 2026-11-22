// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original test doubles for the tailnet node boundary (tests only).

// Package tailnettest provides a fake tailnet node whose listener is a real loopback socket.
package tailnettest

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
)

// HubIPv4 is the fake hub tailnet address (RFC 6598 example range).
const HubIPv4 = "192.0.2.1"

// Node is a controllable tailnet node. Up blocks until Release (when Gate is set) and then returns UpErr.
type Node struct {
	Config tailnet.NodeConfig
	UpErr  error
	Gate   chan struct{}
	Fleet  *Fleet

	mu       sync.Mutex
	closed   bool
	listener net.Listener
}

// Fleet is a shared fake tailnet: status, IPN bus and identities seen by every node it creates.
type Fleet struct {
	mu        sync.Mutex
	status    *ipnstate.Status
	watchers  []*Watcher
	whois     map[string]*apitype.WhoIsResponse
	whoisGate chan struct{}
	statusIn  chan struct{}
	statusOut chan struct{}
	upErrs    []error
	gate      chan struct{}
	nodes     []*Node
	Created   atomic.Int64
	Watches   atomic.Int64
	Refreshes atomic.Int64
}

// NewFleet returns a fleet whose hub is Running at HubIPv4 in tailnet example.com.
func NewFleet() *Fleet {
	return &Fleet{status: RunningStatus(), whois: map[string]*apitype.WhoIsResponse{}}
}

// RunningStatus is a connected hub status without peers.
func RunningStatus() *ipnstate.Status {
	return &ipnstate.Status{
		BackendState:   ipn.Running.String(),
		TailscaleIPs:   []netip.Addr{netip.MustParseAddr(HubIPv4)},
		Self:           &ipnstate.PeerStatus{ID: "hub-node", Online: true},
		CurrentTailnet: &ipnstate.TailnetStatus{Name: "example.com"},
		Peer:           map[key.NodePublic]*ipnstate.PeerStatus{},
	}
}

// SetPeer adds or replaces a peer by stable node ID.
func (f *Fleet) SetPeer(peer ipnstate.PeerStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, p := range f.status.Peer {
		if p.ID == peer.ID {
			delete(f.status.Peer, k)
		}
	}
	f.status.Peer[key.NewNode().Public()] = &peer
}

// RemovePeer removes a peer by stable node ID.
func (f *Fleet) RemovePeer(id tailcfg.StableNodeID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, p := range f.status.Peer {
		if p.ID == id {
			delete(f.status.Peer, k)
		}
	}
}

// FailNextUps makes the next Up calls return these errors in order.
func (f *Fleet) FailNextUps(errs ...error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upErrs = append(f.upErrs, errs...)
}

// HoldUps makes Up block until the returned release function runs.
func (f *Fleet) HoldUps() func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	gate := make(chan struct{})
	f.gate = gate
	return func() { close(gate) }
}

// Factory creates nodes of this fleet.
func (f *Fleet) Factory(cfg tailnet.NodeConfig) (tailnet.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := &Node{Config: cfg, Fleet: f, Gate: f.gate}
	if len(f.upErrs) > 0 {
		n.UpErr, f.upErrs = f.upErrs[0], f.upErrs[1:]
	}
	f.nodes = append(f.nodes, n)
	f.Created.Add(1)
	return n, nil
}

// Nodes returns created nodes.
func (f *Fleet) Nodes() []*Node {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*Node(nil), f.nodes...)
}

// SetStatus replaces the status returned by Status and sent as InitialStatus.
func (f *Fleet) SetStatus(mutate func(*ipnstate.Status)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mutate(f.status)
}

func (f *Fleet) snapshot() *ipnstate.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	clone := *f.status
	clone.Peer = map[key.NodePublic]*ipnstate.PeerStatus{}
	for k, v := range f.status.Peer {
		p := *v
		clone.Peer[k] = &p
	}
	if f.status.Self != nil {
		self := *f.status.Self
		clone.Self = &self
	}
	return &clone
}

// Notify sends a bus message to every open watcher.
func (f *Fleet) Notify(n ipn.Notify) {
	f.mu.Lock()
	watchers := append([]*Watcher(nil), f.watchers...)
	f.mu.Unlock()
	for _, w := range watchers {
		w.send(n)
	}
}

// CloseWatchers ends every open subscription with EOF.
func (f *Fleet) CloseWatchers() {
	f.mu.Lock()
	watchers := f.watchers
	f.watchers = nil
	f.mu.Unlock()
	for _, w := range watchers {
		_ = w.Close()
	}
}

// SetWhoIs maps a peer address host (no port) to a node identity.
func (f *Fleet) SetWhoIs(host, nodeID string, tags ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whois[host] = &apitype.WhoIsResponse{Node: &tailcfg.Node{StableID: tailcfg.StableNodeID(nodeID), Tags: tags}}
}

// HoldWhoIs blocks WhoIs until the returned release function runs.
func (f *Fleet) HoldWhoIs() func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	gate := make(chan struct{})
	f.whoisGate = gate
	return func() { close(gate) }
}

// HoldStatus makes the next Status call take its snapshot, signal entered, and return only after release.
func (f *Fleet) HoldStatus() (entered <-chan struct{}, release func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusIn, f.statusOut = make(chan struct{}), make(chan struct{})
	out := f.statusOut
	return f.statusIn, func() { close(out) }
}

func (n *Node) Up(ctx context.Context) error {
	if n.Gate != nil {
		select {
		case <-n.Gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return n.UpErr
}

// Listen returns a real loopback listener standing in for the tsnet listener.
func (n *Node) Listen(network, _ string) (net.Listener, error) {
	listener, err := net.Listen(network, "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	n.mu.Lock()
	n.listener = listener
	n.mu.Unlock()
	return listener, nil
}

// Addr is the address of the node's ingress listener, once listening.
func (n *Node) Addr() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.listener == nil {
		return ""
	}
	return n.listener.Addr().String()
}

func (n *Node) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

func (n *Node) Status(context.Context) (*ipnstate.Status, error) {
	n.Fleet.Refreshes.Add(1)
	snapshot := n.Fleet.snapshot()
	n.Fleet.mu.Lock()
	in, out := n.Fleet.statusIn, n.Fleet.statusOut
	n.Fleet.statusIn, n.Fleet.statusOut = nil, nil
	n.Fleet.mu.Unlock()
	if in != nil {
		close(in)
		<-out // deliberately ignores cancellation: the reply was already on its way
	}
	return snapshot, nil
}

func (n *Node) Watch(context.Context) (tailnet.Watcher, error) {
	w := &Watcher{ch: make(chan ipn.Notify, 16), done: make(chan struct{})}
	initial := n.Fleet.snapshot()
	state := ipn.State(0)
	for s := ipn.NoState; s <= ipn.Running; s++ {
		if s.String() == initial.BackendState {
			state = s
		}
	}
	w.ch <- ipn.Notify{InitialStatus: initial, State: &state}
	n.Fleet.mu.Lock()
	n.Fleet.watchers = append(n.Fleet.watchers, w)
	n.Fleet.mu.Unlock()
	n.Fleet.Watches.Add(1)
	return w, nil
}

func (n *Node) WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
	n.Fleet.mu.Lock()
	gate := n.Fleet.whoisGate
	n.Fleet.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	n.Fleet.mu.Lock()
	defer n.Fleet.mu.Unlock()
	if who, ok := n.Fleet.whois[host]; ok {
		return who, nil
	}
	return nil, errors.New("no such peer")
}

func (n *Node) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.closed = true
	return nil
}

// Closed reports whether Close ran.
func (n *Node) Closed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.closed
}

// Watcher is a fake IPN bus subscription.
type Watcher struct {
	ch   chan ipn.Notify
	done chan struct{}
	once sync.Once
}

func (w *Watcher) send(n ipn.Notify) {
	select {
	case w.ch <- n:
	case <-w.done:
	}
}

func (w *Watcher) Next() (ipn.Notify, error) {
	select {
	case n := <-w.ch:
		return n, nil
	case <-w.done:
		return ipn.Notify{}, errors.New("EOF")
	}
}

func (w *Watcher) Close() error {
	w.once.Do(func() { close(w.done) })
	return nil
}
