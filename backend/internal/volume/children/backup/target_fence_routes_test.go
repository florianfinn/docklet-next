// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original retire-fence proofs at the hub policy and manual-backup call sites.

package backup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

// parkedRetire holds Retire after its zero-reference answer until release is closed.
type parkedRetire struct {
	atZero, release chan struct{}
	done            chan error
}

func parkRetire(t *testing.T, s *Service, id string) *parkedRetire {
	t.Helper()
	p := &parkedRetire{atZero: make(chan struct{}), release: make(chan struct{}), done: make(chan error, 1)}
	s.targets.ConfigureDelivery(func(context.Context, backuptarget.Target) (dto.TargetReferences, []string, error) {
		close(p.atZero)
		<-p.release
		return dto.TargetReferences{}, nil, nil
	}, nil)
	go func() { p.done <- s.targets.Retire(context.Background(), id) }()
	select {
	case <-p.atZero:
	case <-time.After(5 * time.Second):
		t.Fatal("retire did not reach its reference check")
	}
	return p
}

func initFenceEncryption() {
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
}

// assertFenced proves the admission waits for the parked retire and then fails as retired.
func assertFenced(t *testing.T, p *parkedRetire, admit func() error) {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- admit() }()
	select {
	case err := <-result:
		close(p.release)
		t.Fatalf("admission passed the retire fence while retirement was pending: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(p.release)
	require.NoError(t, <-p.done)
	select {
	case err := <-result:
		var problem *dto.Problem
		require.ErrorAs(t, err, &problem)
		require.Equal(t, "backup.target.retired", problem.Code)
	case <-time.After(5 * time.Second):
		t.Fatal("admission did not finish after retirement")
	}
}

func TestHubLocalPolicyUpdateWaitsForRetireFence(t *testing.T) {
	initFenceEncryption()
	s := targetAdmissionService(t)
	target, err := s.targets.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new("0"), Path: "/srv/backups"}, Name: "target"})
	require.NoError(t, err)
	h := policies.NewHandler(s.policies, nil, nil, t.Context())
	h.AroundUpdate = s.aroundPolicyUpdate(nil, &config.Config{})
	p := parkRetire(t, s, target.ID)
	assertFenced(t, p, func() error {
		_, err := h.UpdateBackupPolicy(t.Context(), &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: "0", VolumeName: "data", Body: volume.UpdateBackupPolicies{Policies: []volume.UpdateBackupPolicy{{TargetID: target.ID, LocalEnabled: true, Schedule: "0 0 2 * * *", RetentionCount: 7}}}})
		return err
	})
	var n int64
	require.NoError(t, s.deps.DB.Model(&policies.VolumeBackupPolicy{}).Count(&n).Error)
	require.Zero(t, n, "a policy referencing a retired target was stored")
}

// remoteFenceHost connects a hub service to a counting agent over Direct or Edge.
type remoteFenceHost struct {
	hub          *Service
	environments *environment.EnvironmentService
	id           string
	effects      *atomic.Int32
	agentDB      *runtimePlan
}

func newRemoteFenceHost(t *testing.T, mode string) *remoteFenceHost {
	t.Helper()
	initFenceEncryption()
	hub, agent := newRuntimePlan(t), newRuntimePlan(t)
	for _, f := range []*runtimePlan{hub, agent} {
		require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}, &policies.VolumeBackupPolicy{}))
		f.service.deps.Store.Latest = func(context.Context, string) (*volume.Backup, error) { return nil, nil }
	}
	token := strings.Repeat("s", 32)
	router, rawAPI := humatest.New(t)
	api := huma.NewGroup(rawAPI, "/api")
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	cfg := &config.Config{AgentMode: mode == "direct", EdgeAgent: mode == "edge", AgentToken: token}
	agentPolicies := policies.NewHandler(agent.service.policies, nil, nil, t.Context())
	agentPolicies.AroundUpdate = agent.service.aroundPolicyUpdate(nil, cfg)
	agentPolicies.BeforeLocalUpdate = agent.service.checkPolicySet
	policies.RegisterRoutes(api, agentPolicies)
	backuptarget.RegisterHostRoutes(api, TargetHandler(agent.service, nil, cfg, t.Context()), true)
	effects := &atomic.Int32{}
	agentHTTP := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/app-version" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(204)
			return
		}
		// Reading the current policy precedes admission; everything else is an effect.
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/backup-policy") {
			effects.Add(1)
		}
		router.ServeHTTP(w, r)
	})
	server := httptest.NewServer(agentHTTP)
	t.Cleanup(server.Close)
	id := uuid.New().String()
	endpoint := server.URL
	if mode == "edge" {
		backupRouteTunnel(t, agentHTTP, edge.GetRegistry(), id, token)
		endpoint = "edge://example"
	}
	require.NoError(t, hub.service.deps.DB.AutoMigrate(&environment.Environment{}))
	require.NoError(t, hub.service.deps.DB.Create(&environment.Environment{BaseModel: database.BaseModel{ID: id}, Name: "Agent", ApiUrl: endpoint, Enabled: true, IsEdge: mode == "edge", AccessToken: &token}).Error)
	return &remoteFenceHost{hub: hub.service, environments: environment.NewEnvironmentService(hub.service.deps.DB, server.Client(), nil, nil, nil, nil), id: id, effects: effects, agentDB: agent}
}

func TestRemoteAdmissionsWaitForRetireFence(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, path := range []string{"policy", "manual-explicit", "manual-policy"} {
			t.Run(mode+"/"+path, func(t *testing.T) {
				f := newRemoteFenceHost(t, mode)
				target, err := f.hub.targets.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new(f.id), Path: "/srv/backups"}, Name: "target"})
				require.NoError(t, err)
				if path == "manual-policy" {
					require.NoError(t, f.agentDB.service.deps.DB.Create(&policies.VolumeBackupPolicy{VolumeName: "data", LocalEnabled: true, TargetID: target.ID, Schedule: "0 0 2 * * *", RetentionCount: 7}).Error)
				}
				ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.Actor{ID: "user", Username: "admin"})
				admit := func() error {
					if path == "policy" {
						h := policies.NewHandler(f.hub.policies, nil, f.environments, t.Context())
						h.AroundUpdate = f.hub.aroundPolicyUpdate(f.environments, &config.Config{})
						_, err := h.UpdateBackupPolicy(ctx, &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: f.id, VolumeName: "data", Body: volume.UpdateBackupPolicies{Policies: []volume.UpdateBackupPolicy{{TargetID: target.ID, LocalEnabled: true, Schedule: "0 0 2 * * *", RetentionCount: 7}}}})
						return err
					}
					request := &volume.CreateBackupRequest{}
					if path == "manual-explicit" {
						request.TargetID = target.ID
					}
					_, err := NewHandler(f.hub, nil, f.environments, nil, t.Context(), &config.Config{}).CreateBackup(ctx, &CreateBackupInput{EnvironmentID: f.id, VolumeName: "data", Body: request})
					return err
				}
				p := parkRetire(t, f.hub, target.ID)
				assertFenced(t, p, admit)
				require.Zero(t, f.effects.Load(), "a fenced admission reached the owning agent")
			})
		}
	}
}
