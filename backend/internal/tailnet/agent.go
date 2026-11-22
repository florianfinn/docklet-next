// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original agent-side tailnet join and manager dialer for the tailnet route.

package tailnet

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

// AgentConfig is the agent's env-only tailnet configuration. The route is on when AuthKey is set;
// after the first join the retained state is used and the consumed key is ignored.
type AgentConfig struct {
	// Requested is the operator's opt-in (a key or an explicit TAILSCALE_HOSTNAME), decided before defaults apply.
	Requested  bool
	AuthKey    string
	Hostname   string
	StateDir   string
	ControlURL string
	Factory    NodeFactory
	RetryMin   time.Duration // 5 s
	RetryMax   time.Duration // 60 s
}

// Enabled reports whether the agent uses the tailnet route: an operator opt-in, a first-join key or a
// retained node identity. A restart without the consumed key keeps the route.
func (c AgentConfig) Enabled() bool {
	return c.Requested || strings.TrimSpace(c.AuthKey) != "" || HasNodeState(c.StateDir)
}

// Agent is the agent's joined node.
type Agent struct {
	node Node
}

// JoinAgent joins the tailnet before any hub request, retrying with backoff until ctx ends.
// Agents never advertise tags: the one-use key carries tag:docklet-agent.
func JoinAgent(ctx context.Context, cfg AgentConfig) (*Agent, error) {
	if cfg.Factory == nil {
		cfg.Factory = NewTSNetNode
	}
	if cfg.RetryMin <= 0 {
		cfg.RetryMin = 5 * time.Second
	}
	if cfg.RetryMax <= 0 {
		cfg.RetryMax = 60 * time.Second
	}
	delay := cfg.RetryMin
	for attempt := 1; ; attempt++ {
		// A plain auth key needs no network exchange and a logged-in node ignores it, so the configured key is
		// always offered; an interrupted first join keeps it for the next attempt.
		key := strings.TrimSpace(cfg.AuthKey)
		if key == "" && !HasNodeState(cfg.StateDir) {
			slog.ErrorContext(ctx, "Tailnet node state is missing and no auth key is configured; re-add this host to issue a new key", "attempt", attempt)
		}
		node, err := cfg.Factory(NodeConfig{
			Dir:        cfg.StateDir,
			Hostname:   cfg.Hostname,
			ControlURL: cfg.ControlURL,
			AuthKey:    key,
			Secrets:    []string{cfg.AuthKey},
		})
		if err == nil {
			upCtx, cancel := context.WithTimeout(ctx, cfg.RetryMax)
			err = node.Up(upCtx)
			cancel()
			if err == nil {
				slog.InfoContext(ctx, "Joined the tailnet", "attempt", attempt, "hostname", cfg.Hostname)
				return &Agent{node: node}, nil
			}
			_ = node.Close()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		slog.WarnContext(ctx, "Tailnet join failed; retrying", "attempt", attempt, "retryIn", delay, "error", redactSecrets(err.Error(), cfg.AuthKey))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, cfg.RetryMax)
	}
}

// Close leaves the tailnet and keeps the node state.
func (a *Agent) Close() error {
	if a == nil || a.node == nil {
		return nil
	}
	return a.node.Close()
}

// AgentRoute late-binds the joined node behind a dialer that exists before any service starts.
type AgentRoute struct {
	cfg   AgentConfig
	agent atomic.Pointer[Agent]
}

// NewAgentRoute returns a route; Enabled is false when no key, tailnet hostname or retained identity exists.
func NewAgentRoute(cfg AgentConfig) *AgentRoute { return &AgentRoute{cfg: cfg} }

func (r *AgentRoute) Enabled() bool { return r != nil && r.cfg.Enabled() }

// Join blocks until the node is in the tailnet or ctx ends.
func (r *AgentRoute) Join(ctx context.Context) error {
	agent, err := JoinAgent(ctx, r.cfg)
	if err != nil {
		return err
	}
	r.agent.Store(agent)
	return nil
}

// Dial carries hub connections; before the first join it fails instead of falling back to normal routing.
func (r *AgentRoute) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	agent := r.agent.Load()
	if agent == nil {
		return nil, errNotJoined
	}
	return agent.node.Dial(ctx, network, address)
}

var errNotJoined = errors.New("the agent has not joined the tailnet yet")

// Close leaves the tailnet and keeps the node state.
func (r *AgentRoute) Close() error {
	if r == nil {
		return nil
	}
	return r.agent.Swap(nil).Close()
}
