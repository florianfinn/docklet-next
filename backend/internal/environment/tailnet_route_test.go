// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original tailnet route projection, observation fencing and onboarding tests.

package environment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/types/environment"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

type fakeTailnetRoute struct {
	mu       sync.Mutex
	conn     tailnettypes.Connection
	refusal  error
	key      string
	keyErr   error
	pasted   string
	whoGate  chan struct{}
	whoNode  map[string]string
	whoTag   bool
	lookups  int
	peerSeen string
	mints    int
}

func (f *fakeTailnetRoute) Status() tailnettypes.Status {
	return tailnettypes.Status{Enabled: true, Connection: f.Connection()}
}
func (f *fakeTailnetRoute) RouteRefusal() error { return f.refusal }
func (f *fakeTailnetRoute) ProvisionHostKey(_ context.Context, pasted string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pasted = pasted
	f.mints++
	if f.keyErr != nil {
		return "", "", f.keyErr
	}
	return f.key, "http://192.0.2.1:3552", nil
}
func (f *fakeTailnetRoute) Connection() tailnettypes.Connection {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conn
}
func (f *fakeTailnetRoute) HostPeer(nodeID string, tagged *bool, observedAt *time.Time) tailnettypes.HostPeer {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peerSeen = nodeID
	state := "unknown"
	if nodeID != "" {
		state = "offline"
	}
	return tailnettypes.HostPeer{State: state, Path: "unknown", Tagged: tagged, ObservedAt: observedAt}
}
func (f *fakeTailnetRoute) WhoIs(ctx context.Context, remoteAddr string) (string, bool, error) {
	if f.whoGate != nil {
		select {
		case <-f.whoGate:
		case <-ctx.Done():
			return "", false, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups++
	node, ok := f.whoNode[remoteAddr]
	if !ok {
		return "", false, errors.New("unknown peer")
	}
	return node, f.whoTag, nil
}

func tailnetHost(t *testing.T, db *database.DB, id string) {
	t.Helper()
	createTestEnvironmentWithState(t, db, id, "edge://"+id, string(EnvironmentStatusOffline), true, nil)
	require.NoError(t, db.WithContext(t.Context()).Model(&Environment{}).Where("id = ?", id).Update("network_route", tailnettypes.RouteTailnet).Error)
}

func observed(t *testing.T, db *database.DB, id string) Environment {
	t.Helper()
	var row Environment
	require.NoError(t, db.WithContext(t.Context()).First(&row, "id = ?", id).Error)
	return row
}

func TestTailnetObservationRecordsTheAuthenticatedNode(t *testing.T) {
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)
	route := &fakeTailnetRoute{whoNode: map[string]string{"198.51.100.2:41000": "agent-a", "198.51.100.3:41000": "agent-b"}, whoTag: true}
	svc.SetTailnet(route)
	tailnetHost(t, db, "host-1")

	svc.ObserveTailnetPeer(t.Context(), "host-1")
	svc.WaitTailnetObservations()
	require.Zero(t, route.lookups, "requests outside the tailnet ingress are not observed")

	svc.ObserveTailnetPeer(tailnet.WithPeerAddress(t.Context(), "198.51.100.2:41000"), "host-1")
	svc.WaitTailnetObservations()
	row := observed(t, db, "host-1")
	require.Equal(t, "agent-a", *row.TailnetNodeID)
	require.True(t, *row.TailnetTagged)
	require.NotNil(t, row.TailnetObservedAt)

	// A different node presenting the host's valid token updates the observation (diagnostic, not a rebind).
	route.whoTag = false
	svc.ObserveTailnetPeer(tailnet.WithPeerAddress(t.Context(), "198.51.100.3:41000"), "host-1")
	svc.WaitTailnetObservations()
	row = observed(t, db, "host-1")
	require.Equal(t, "agent-b", *row.TailnetNodeID)
	require.False(t, *row.TailnetTagged, "a node without tag:docklet-agent is stored as untagged")
}

// A lookup that outlives a token rotation or the host's removal must not write.
func TestTailnetObservationIsFencedByTheRegistrationRevision(t *testing.T) {
	for _, change := range []string{"token rotation", "host removal"} {
		t.Run(change, func(t *testing.T) {
			db := setupEnvironmentServiceTestDB(t)
			svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)
			route := &fakeTailnetRoute{whoNode: map[string]string{"198.51.100.2:41000": "agent-a"}, whoTag: true, whoGate: make(chan struct{})}
			svc.SetTailnet(route)
			tailnetHost(t, db, "host-1")

			svc.ObserveTailnetPeer(tailnet.WithPeerAddress(t.Context(), "198.51.100.2:41000"), "host-1")
			switch change {
			case "token rotation":
				require.NoError(t, db.Exec("UPDATE environments SET credential_revision = credential_revision + 1 WHERE id = ?", "host-1").Error)
			case "host removal":
				require.NoError(t, db.Exec("DELETE FROM environments WHERE id = ?", "host-1").Error)
			}
			close(route.whoGate)
			svc.WaitTailnetObservations()

			var rows []Environment
			require.NoError(t, db.Find(&rows, "id = ?", "host-1").Error)
			if change == "host removal" {
				require.Empty(t, rows, "a late observation never recreates a removed host")
				return
			}
			require.Len(t, rows, 1)
			require.Nil(t, rows[0].TailnetNodeID, "the stale lookup is discarded")
		})
	}
}

func TestTailnetProjectionAddsPeerAndFirstFailingLink(t *testing.T) {
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)
	route := &fakeTailnetRoute{conn: tailnettypes.Connection{State: "connected"}}
	svc.SetTailnet(route)
	tailnetHost(t, db, "host-1")
	createTestEnvironmentWithState(t, db, "host-lan", "edge://host-lan", string(EnvironmentStatusOffline), true, nil)
	require.NoError(t, db.Model(&Environment{}).Where("id = ?", "host-lan").Update("network_route", tailnettypes.RouteLAN).Error)
	_, err := svc.recordTailnetObservationInternal(t.Context(), "host-1", 0, "agent-a", true, time.Now())
	require.NoError(t, err)

	dto := environment.Environment{ID: "host-1", IsEdge: true}
	svc.ApplyHostRuntimeState(t.Context(), &dto)
	require.Equal(t, tailnettypes.RouteTailnet, *dto.NetworkRoute)
	require.NotNil(t, dto.Tailnet)
	require.Equal(t, "agent-a", route.peerSeen)
	require.Equal(t, "offline", dto.Tailnet.State)
	require.Equal(t, "peer", dto.Tailnet.FailingLink)

	route.mu.Lock()
	route.conn = tailnettypes.Connection{State: "off"}
	route.mu.Unlock()
	svc.ApplyHostRuntimeState(t.Context(), &dto)
	require.Equal(t, "hub", dto.Tailnet.FailingLink, "a disabled hub route is link 1")

	lan := environment.Environment{ID: "host-lan", IsEdge: true}
	svc.ApplyHostRuntimeState(t.Context(), &lan)
	require.Equal(t, tailnettypes.RouteLAN, *lan.NetworkRoute)
	require.Nil(t, lan.Tailnet)
}

func newTailnetHandler(t *testing.T, mtls string) (*EnvironmentHandler, *database.DB, *fakeTailnetRoute) {
	t.Helper()
	db := setupEnvironmentServiceTestDB(t)
	keys := apikey.NewApiKeyService(db, user.NewUserService(db, nil, nil), nil)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, keys)
	route := &fakeTailnetRoute{conn: tailnettypes.Connection{State: "connected"}, key: "tskey-auth-example-host"}
	svc.SetTailnet(route)
	return NewHandler(svc, nil, keys, nil, &config.Config{EdgeMTLSMode: mtls}, nil), db, route
}

func operatorContext(t *testing.T) context.Context {
	return context.WithValue(t.Context(), usertypes.CurrentUserContextKey{}, &usertypes.Actor{ID: "operator", Username: "operator"})
}

func TestCreateTailnetHostReturnsTheOneUseKeyOnce(t *testing.T) {
	h, db, _ := newTailnetHandler(t, "disabled")
	route, isEdge, useKey := tailnettypes.RouteTailnet, true, true
	name := "edge-example"
	out, err := h.CreateEnvironment(operatorContext(t), &CreateEnvironmentInput{Body: environment.Create{ApiUrl: "edge://example", Name: &name, IsEdge: &isEdge, UseApiKey: &useKey, Route: &route}})
	require.NoError(t, err)
	require.Equal(t, "tskey-auth-example-host", *out.Body.Data.TailnetAuthKey)
	require.Equal(t, "http://192.0.2.1:3552", *out.Body.Data.TailnetManagerURL)
	require.Equal(t, tailnettypes.RouteTailnet, *out.Body.Data.NetworkRoute)
	// The wire body is what the installer reads: the custom marshaler must carry the grant next to the pairing key.
	var wire struct {
		Data map[string]any `json:"data"`
	}
	body, err := json.Marshal(out.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &wire))
	require.Equal(t, "tskey-auth-example-host", wire.Data["tailnetAuthKey"])
	require.Equal(t, "http://192.0.2.1:3552", wire.Data["tailnetManagerUrl"])
	require.NotEmpty(t, wire.Data["apiKey"])
	require.Equal(t, "tailnet", wire.Data["networkRoute"])
	stored := observed(t, db, out.Body.Data.ID)
	require.Equal(t, tailnettypes.RouteTailnet, *stored.NetworkRoute)

	read, err := h.GetEnvironment(operatorContext(t), &GetEnvironmentInput{ID: out.Body.Data.ID})
	require.NoError(t, err)
	require.Nil(t, read.Body.Data.ApiKey, "the key is never part of later reads")
}

// A refused or failed provisioning leaves no host row and no key behind.
func TestCreateTailnetHostRefusalsStoreNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		keyErr error
		code   string
	}{
		{"hub not connected", &tailnettypes.Problem{Code: tailnettypes.CodeNotConnected, Status: 409}, tailnettypes.CodeNotConnected},
		{"mTLS mode", &tailnettypes.Problem{Code: tailnettypes.CodeMTLSUnsupported, Status: 409}, tailnettypes.CodeMTLSUnsupported},
		{"provider refused", &tailnettypes.Problem{Code: tailnettypes.CodeCredentialRejected, Status: 409, ProviderStatus: 403}, tailnettypes.CodeCredentialRejected},
		{"mint failed", &tailnettypes.Problem{Code: tailnettypes.CodeKeyMintFailed, Status: 502, ProviderStatus: 503}, tailnettypes.CodeKeyMintFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, db, fake := newTailnetHandler(t, "disabled")
			fake.keyErr = tc.keyErr
			route, isEdge, useKey, name := tailnettypes.RouteTailnet, true, true, "edge-refused"
			_, err := h.CreateEnvironment(operatorContext(t), &CreateEnvironmentInput{Body: environment.Create{ApiUrl: "edge://example", Name: &name, IsEdge: &isEdge, UseApiKey: &useKey, Route: &route}})
			var problem *tailnettypes.Problem
			require.ErrorAs(t, err, &problem)
			require.Equal(t, tc.code, problem.Code)
			var count int64
			require.NoError(t, db.Model(&Environment{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
	// A refused name is checked before a one-use key is minted (review question Q-C).
	t.Run("invalid or taken name mints no key", func(t *testing.T) {
		h, db, fake := newTailnetHandler(t, "disabled")
		tailnetHost(t, db, "existing")
		var existing Environment
		require.NoError(t, db.First(&existing, "id = ?", "existing").Error)
		route, isEdge, useKey := tailnettypes.RouteTailnet, true, true
		for _, name := range []string{"", strings.Repeat("n", 41), existing.Name} {
			_, err := h.CreateEnvironment(operatorContext(t), &CreateEnvironmentInput{Body: environment.Create{ApiUrl: "edge://example", Name: &name, IsEdge: &isEdge, UseApiKey: &useKey, Route: &route}})
			var problem *environment.HostProblem
			require.ErrorAs(t, err, &problem, name)
		}
		require.Zero(t, fake.mints, "no key is minted for a refused name")
	})
	t.Run("manual key forwarded unchanged and route only for Edge", func(t *testing.T) {
		h, _, fake := newTailnetHandler(t, "disabled")
		route, isEdge, useKey, pasted, name := tailnettypes.RouteTailnet, true, true, "pasted-host-key", "edge-manual"
		_, err := h.CreateEnvironment(operatorContext(t), &CreateEnvironmentInput{Body: environment.Create{ApiUrl: "edge://example", Name: &name, IsEdge: &isEdge, UseApiKey: &useKey, Route: &route, TailnetAuthKey: &pasted}})
		require.NoError(t, err)
		require.Equal(t, pasted, fake.pasted)
		direct := false
		_, err = h.CreateEnvironment(operatorContext(t), &CreateEnvironmentInput{Body: environment.Create{ApiUrl: "http://agent.example.com:3552", IsEdge: &direct, UseApiKey: &useKey, Route: &route}})
		require.Error(t, err)
	})
}
