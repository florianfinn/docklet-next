// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original hub service that binds the stored credential to the tailnet node lifecycle.

package tailnet

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

// HostCounter reports the number of tailnet-route hosts and how many of them are online.
type HostCounter func(ctx context.Context) (total, online int)

// Service is the hub's tailnet route: stored credential, node lifecycle and status.
type Service struct {
	manager    *Manager
	store      *Store
	hostname   string
	controlURL string

	writes sync.Mutex
	mu     sync.Mutex
	counts HostCounter
}

// NewService builds the hub service; agents get nil and use JoinAgent instead.
func NewService(cfg *config.Config, settings SettingsBackend) *Service {
	if cfg == nil || cfg.AgentMode || cfg.EdgeAgent {
		return nil
	}
	manager := NewManager(ManagerConfig{
		StateDir:     cfg.TailscaleStateDir,
		Hostname:     cmpOr(cfg.TailscaleHostname, "docklet-hub"),
		ControlURL:   strings.TrimSpace(cfg.TailscaleControlURL),
		MTLSDisabled: edge.NormalizeEdgeMTLSMode(cfg.EdgeMTLSMode) == edge.EdgeMTLSModeDisabled,
		Provisioner:  &Provisioner{},
	})
	// The raw value decides configuredness and installer use; only the hub's own join trims it.
	controlURL := cfg.TailscaleControlURL
	if _, ok := InstallerControlURL(controlURL); controlURL != "" && !ok {
		// Never log the value: it may carry credentials.
		slog.Warn("TAILSCALE_CONTROL_URL is not an http(s) URL without credentials; new hosts cannot use the Tailscale route")
	}
	return newService(manager, NewStore(settings), cmpOr(cfg.TailscaleHostname, "docklet-hub"), controlURL)
}

func newService(manager *Manager, store *Store, hostname, controlURL string) *Service {
	return &Service{manager: manager, store: store, hostname: hostname, controlURL: controlURL}
}

func cmpOr(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

// Manager exposes the node lifecycle to the host projection and ingress wiring.
func (s *Service) Manager() *Manager {
	if s == nil {
		return nil
	}
	return s.manager
}

// SetHostCounter installs the host count source used by status reads.
func (s *Service) SetHostCounter(counter HostCounter) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts = counter
}

// Start applies the stored state once at hub start.
func (s *Service) Start(ctx context.Context) {
	if s == nil {
		return
	}
	enabled, cred, err := s.store.Load(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Tailnet route stays off", "error", err)
		return
	}
	s.manager.Apply(ctx, enabled, cred)
}

// Stop closes the node and keeps its state.
func (s *Service) Stop(ctx context.Context) {
	if s != nil {
		s.manager.Close(ctx)
	}
}

// Status is the hub status with host counts.
func (s *Service) Status(ctx context.Context) tailnettypes.Status {
	status := s.manager.Status()
	s.mu.Lock()
	counter := s.counts
	s.mu.Unlock()
	if counter != nil && status.Enabled {
		status.HostCount, status.OnlineHosts = counter(ctx)
	}
	return status
}

// Settings is the write-only read model.
func (s *Service) Settings(ctx context.Context) (tailnettypes.Settings, error) {
	enabled, cred, err := s.store.Load(ctx)
	if err != nil {
		return tailnettypes.Settings{}, err
	}
	return ReadModel(enabled, cred, s.hostname, s.controlURL), nil
}

// Update stores the credential and switch, then restarts the node in a new generation.
func (s *Service) Update(ctx context.Context, update tailnettypes.SettingsUpdate) (tailnettypes.Settings, error) {
	s.writes.Lock()
	defer s.writes.Unlock()
	_, stored, err := s.store.Load(ctx)
	if err != nil {
		stored = Credential{}
	}
	next, err := Merge(stored, update)
	if err != nil {
		return tailnettypes.Settings{}, err
	}
	if err := s.store.Save(ctx, update.Enabled, next); err != nil {
		return tailnettypes.Settings{}, err
	}
	s.manager.Apply(ctx, update.Enabled, next)
	return ReadModel(update.Enabled, next, s.hostname, s.controlURL), nil
}

// Reset turns the route off and deletes the hub's local node identity; the credential is kept.
func (s *Service) Reset(ctx context.Context) (tailnettypes.Status, error) {
	s.writes.Lock()
	defer s.writes.Unlock()
	_, cred, err := s.store.Load(ctx)
	if err != nil {
		cred = Credential{}
	}
	if err := s.store.Save(ctx, false, cred); err != nil {
		return tailnettypes.Status{}, err
	}
	if err := s.manager.Reset(ctx); err != nil {
		return tailnettypes.Status{}, err
	}
	return s.Status(ctx), nil
}
