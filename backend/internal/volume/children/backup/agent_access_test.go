// Source: docklet-next/issues/283
// Revision: e5218d798f5302f7fa568a6059a70901bcc83219
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original central backup-route refusal of an outdated or refused Direct or Edge agent.

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

type outdatedAgent struct {
	f            *runtimePlan
	environments *environment.EnvironmentService
	id           string
	ctx          context.Context
	forwarded    *int
}

// newOutdatedAgent registers a Direct or Edge host whose agent reports the
// previous contract and counts every request beyond the version probe.
func newOutdatedAgent(t *testing.T, mode string) outdatedAgent {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	t.Cleanup(cancel)
	f := newRuntimePlan(t)
	require.NoError(t, f.service.deps.DB.AutoMigrate(&environment.Environment{}, &backuptarget.Target{}))
	info, _ := json.Marshal(version.Info{AgentContractVersion: contract.AgentHubVersion - 1, AgentContractFingerprint: "previous", MinimumAgentContractVersion: contract.AgentHubVersion - 1})
	forwarded := new(int)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/app-version" {
			_, _ = w.Write(info)
			return
		}
		if r.Method == http.MethodHead && strings.HasSuffix(r.URL.Path, "/system/health") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		*forwarded++
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	t.Cleanup(remote.Close)
	id, token, endpoint := "outdated-host", strings.Repeat("o", 32), remote.URL
	if mode == "edge" {
		endpoint = "edge://example"
		conn := &backupIdentityConn{}
		tunnel := edge.NewAgentTunnelWithConn(id, conn)
		tunnel.TokenHash = edge.AgentTokenHash(token)
		conn.send = func(m *edge.TunnelMessage) {
			if m.Type != edge.MessageTypeCommandRequest {
				return
			}
			pending, ok := tunnel.Pending.Load(m.ID)
			require.True(t, ok)
			body := []byte(`{"success":true,"data":{}}`)
			if m.Path == "/api/app-version" {
				body = info
			} else {
				*forwarded++
			}
			pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: 200, Body: body}
		}
		edge.GetRegistry().Register(id, tunnel)
		t.Cleanup(func() { edge.GetRegistry().Unregister(id) })
	}
	require.NoError(t, f.service.deps.DB.Create(&environment.Environment{BaseModel: database.BaseModel{ID: id}, Name: "Example", Enabled: true, IsEdge: mode == "edge", ApiUrl: endpoint, AccessToken: &token}).Error)
	environments := environment.NewEnvironmentService(f.service.deps.DB, remote.Client(), nil, nil, nil, nil)
	return outdatedAgent{f: f, environments: environments, id: id, ctx: context.WithValue(ctx, user.CurrentUserContextKey{}, &user.SystemUser), forwarded: forwarded}
}

func requireAgentAccessRefusal(t *testing.T, err error, status int, code string) {
	t.Helper()
	require.Error(t, err)
	typed, ok := err.(interface{ GetStatus() int })
	require.True(t, ok, "refusal is a typed HTTP status, not an internal error: %v", err)
	require.Equal(t, status, typed.GetStatus())
	body, e := json.Marshal(err)
	require.NoError(t, e)
	require.JSONEq(t, `{"success":false,"code":"`+code+`"}`, string(body))
}

func TestCentralBackupRoutesRefuseOutdatedAgentWithCompatibilityCode(t *testing.T) {
	routes := map[string]func(t *testing.T, a outdatedAgent) error{
		"create/default": func(t *testing.T, a outdatedAgent) error {
			handler := &Handler{service: a.f.service, environmentService: a.environments, appCtx: t.Context()}
			_, err := handler.CreateBackup(a.ctx, &CreateBackupInput{EnvironmentID: a.id, VolumeName: "data", Body: &volume.CreateBackupRequest{}})
			return err
		},
		"create/local": func(t *testing.T, a outdatedAgent) error {
			handler := &Handler{service: a.f.service, environmentService: a.environments, appCtx: t.Context()}
			_, err := handler.CreateBackup(a.ctx, &CreateBackupInput{EnvironmentID: a.id, VolumeName: "data", Body: &volume.CreateBackupRequest{TargetID: "local"}})
			return err
		},
		"policy/get": func(t *testing.T, a outdatedAgent) error {
			_, err := policies.NewHandler(a.f.service.policies, nil, a.environments, t.Context()).GetBackupPolicy(a.ctx, &policies.GetVolumeBackupPolicyInput{EnvironmentID: a.id, VolumeName: "data"})
			return err
		},
		"policy/put": func(t *testing.T, a outdatedAgent) error {
			h := policies.NewHandler(a.f.service.policies, nil, a.environments, t.Context())
			h.AroundUpdate = a.f.service.aroundPolicyUpdate(a.environments, &config.Config{})
			_, err := h.UpdateBackupPolicy(a.ctx, &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: a.id, VolumeName: "data", Body: volume.UpdateBackupPolicies{Policies: []volume.UpdateBackupPolicy{{TargetID: "local"}}}})
			return err
		},
		"target/test": func(t *testing.T, a outdatedAgent) error {
			crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
			target, err := a.f.service.targets.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new(a.id), Path: "/srv/backups"}, Name: "archive"})
			require.NoError(t, err)
			_, err = TargetHandler(a.f.service, a.environments, &config.Config{}, t.Context()).TestTarget(a.ctx, &backuptarget.TestInput{ID: target.ID, Body: dto.TargetTestRequest{HostID: a.id}})
			return err
		},
	}
	for _, mode := range []string{"direct", "edge"} {
		for name, route := range routes {
			t.Run(mode+"/"+name, func(t *testing.T) {
				a := newOutdatedAgent(t, mode)
				requireAgentAccessRefusal(t, route(t, a), http.StatusConflict, "agent.update-required")
				require.Zero(t, *a.forwarded, "no backup or target request reaches an outdated agent")
			})
		}
	}
}

func TestRemoteBackupJSONMapsUpstreamAuthenticationRefusal(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		proxy := func(context.Context, string, string, string, []byte, any) error {
			return &remenv.StatusError{StatusCode: status, Body: []byte(`{"detail":"PRIVATE"}`)}
		}
		_, err := backuptarget.RemoteJSON[map[string]any](t.Context(), proxy, "host", http.MethodGet, "/api/environments/0/volumes/backups/overview", nil)
		requireAgentAccessRefusal(t, err, http.StatusBadGateway, "agent.invalid-token")
	}
	proxy := func(context.Context, string, string, string, []byte, any) error {
		return &remenv.StatusError{StatusCode: http.StatusNotFound}
	}
	_, err := backuptarget.RemoteJSON[map[string]any](t.Context(), proxy, "host", http.MethodGet, "/api/environments/0/volumes/backups/overview", nil)
	var access *backuptarget.AgentAccessError
	require.False(t, errors.As(err, &access), "other upstream statuses keep their own translation")
}
