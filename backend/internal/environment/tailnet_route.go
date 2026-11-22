// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original host-side projection and diagnostic node observation of the tailnet route.

package environment

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/types/environment"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

// TailnetRoute is the hub tailnet surface the host module consumes.
type TailnetRoute interface {
	Status() tailnettypes.Status
	RouteRefusal() error
	ProvisionHostKey(ctx context.Context, pasted string) (string, string, error)
	Connection() tailnettypes.Connection
	HostPeer(nodeID string, tagged *bool, observedAt *time.Time) tailnettypes.HostPeer
	WhoIs(ctx context.Context, remoteAddr string) (string, bool, error)
}

// observationInterval bounds WhoIs lookups per host and peer address; polls authenticate every few seconds.
const observationInterval = time.Minute

type tailnetState struct {
	mu      sync.Mutex
	route   TailnetRoute
	seen    map[string]time.Time
	pending sync.WaitGroup
}

// SetTailnet installs the hub route; nil keeps tailnet hosts projected without peer data.
func (s *EnvironmentService) SetTailnet(route TailnetRoute) {
	s.tailnet.mu.Lock()
	defer s.tailnet.mu.Unlock()
	s.tailnet.route = route
}

func (s *EnvironmentService) tailnetRoute() TailnetRoute {
	s.tailnet.mu.Lock()
	defer s.tailnet.mu.Unlock()
	return s.tailnet.route
}

// applyTailnetProjectionInternal adds the route and, for tailnet hosts, the peer view and first failing link.
func (s *EnvironmentService) applyTailnetProjectionInternal(env *environment.Environment, row *Environment) {
	env.NetworkRoute = row.NetworkRoute
	env.Tailnet = nil
	route := s.tailnetRoute()
	if route == nil || row.NetworkRoute == nil || *row.NetworkRoute != tailnettypes.RouteTailnet {
		return
	}
	nodeID := ""
	if row.TailnetNodeID != nil {
		nodeID = *row.TailnetNodeID
	}
	peer := route.HostPeer(nodeID, row.TailnetTagged, row.TailnetObservedAt)
	peer.FailingLink = tailnet.FailingLink(route.Connection(), peer.State, env.Status)
	env.Tailnet = &peer
}

// CountTailnetHosts counts tailnet-route hosts and those with an online projected status.
func (s *EnvironmentService) CountTailnetHosts(ctx context.Context) (int, int) {
	var rows []Environment
	if err := s.db.WithContext(ctx).Where("network_route = ?", tailnettypes.RouteTailnet).Find(&rows).Error; err != nil {
		return 0, 0
	}
	online := 0
	for i := range rows {
		dto := environment.Environment{ID: rows[i].ID, IsEdge: rows[i].IsEdge, Status: rows[i].Status}
		s.ApplyHostRuntimeState(ctx, &dto)
		if dto.Status == string(EnvironmentStatusOnline) || dto.Status == string(EnvironmentStatusStandby) {
			online++
		}
	}
	return len(rows), online
}

// ObserveTailnetPeer records which tailnet node presented a host's valid credential. It is
// diagnostic only and runs after authentication; the write is fenced by the host's registration revision.
func (s *EnvironmentService) ObserveTailnetPeer(ctx context.Context, envID string) {
	addr, ok := tailnet.PeerAddress(ctx)
	route := s.tailnetRoute()
	if !ok || route == nil || envID == "" {
		return
	}
	now := time.Now()
	key := envID + "|" + addr
	s.tailnet.mu.Lock()
	if s.tailnet.seen == nil {
		s.tailnet.seen = map[string]time.Time{}
	}
	if last, found := s.tailnet.seen[key]; found && now.Sub(last) < observationInterval {
		s.tailnet.mu.Unlock()
		return
	}
	s.tailnet.seen[key] = now
	s.tailnet.mu.Unlock()

	row, err := s.GetEnvironmentByID(ctx, envID)
	if err != nil {
		return
	}
	revision := row.CredentialRevision
	s.tailnet.pending.Add(1)
	go func() {
		defer s.tailnet.pending.Done()
		lookupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		nodeID, tagged, err := route.WhoIs(lookupCtx, addr)
		if err != nil {
			slog.DebugContext(lookupCtx, "Tailnet peer lookup failed", "environmentId", envID, "error", err)
			s.forgetTailnetObservationInternal(key)
			return
		}
		if _, err := s.recordTailnetObservationInternal(lookupCtx, envID, revision, nodeID, tagged, time.Now()); err != nil {
			slog.WarnContext(lookupCtx, "Tailnet observation not stored", "environmentId", envID, "error", err)
		}
	}()
}

func (s *EnvironmentService) forgetTailnetObservationInternal(key string) {
	s.tailnet.mu.Lock()
	defer s.tailnet.mu.Unlock()
	delete(s.tailnet.seen, key)
}

// recordTailnetObservationInternal writes only while the registration revision is unchanged,
// so a lookup that outlives a token rotation or host removal is discarded.
func (s *EnvironmentService) recordTailnetObservationInternal(ctx context.Context, envID string, revision int64, nodeID string, tagged bool, at time.Time) (bool, error) {
	result := s.db.WithContext(ctx).Model(&Environment{}).
		Where("id = ? AND credential_revision = ?", envID, revision).
		UpdateColumns(map[string]any{"tailnet_node_id": nodeID, "tailnet_tagged": tagged, "tailnet_observed_at": at.UTC()})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	s.invalidateEnvironmentCacheInternal(envID)
	s.NotifyRuntimeStateChanged()
	return true, nil
}

// WaitTailnetObservations waits for in-flight observation lookups.
func (s *EnvironmentService) WaitTailnetObservations() { s.tailnet.pending.Wait() }
