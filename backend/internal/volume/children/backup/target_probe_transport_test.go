// Source: docklet-next/issues/289
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backup

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDraftProbeThroughDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newRuntimePlan(t)
			require.NoError(t, f.service.deps.DB.Exec("INSERT INTO settings(key,value) VALUES ('hubHostId','0')").Error)
			require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}))
			router, raw := humatest.New(t)
			api := huma.NewGroup(raw, "/api")
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
			})
			backuptarget.RegisterHostRoutes(api, TargetHandler(f.service, nil, &config.Config{AgentMode: true}, t.Context()), true)
			call := backupRouteTransport(t, mode, router)
			for _, kind := range []string{"smb", "local", "mismatch"} {
				target := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Draft"}
				if kind != "smb" {
					target = dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new("0"), Path: "/srv/archive"}, Name: "Draft"}
				}
				host := "0"
				if kind == "mismatch" {
					host = "host-a"
				}
				payload, err := json.Marshal(dto.TargetProbeRequest{Target: target, ProbeHostID: host})
				require.NoError(t, err)
				before := len(f.creates)
				status, _, body := call(t.Context(), "POST", "/api/environments/0/volumes/backups/targets/probe", payload)
				if kind == "mismatch" {
					require.Equal(t, 422, status, string(body))
					require.Contains(t, string(body), "backup.target.probe-host-mismatch")
					require.Len(t, f.creates, before)
				} else {
					require.Equal(t, 200, status, string(body))
					require.Contains(t, string(body), `"ok":true`)
				}
			}
			var count int64
			require.NoError(t, f.service.deps.DB.Model(&backuptarget.Target{}).Count(&count).Error)
			require.Zero(t, count)
			require.NoError(t, f.service.deps.DB.Model(&backuptarget.Check{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestDraftProbeFromHubThroughDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newRemoteFenceHost(t, mode)
			require.NoError(t, f.agentDB.service.deps.DB.Exec("INSERT INTO settings(key,value) VALUES ('hubHostId',?)", f.id).Error)
			h := TargetHandler(f.hub, f.environments, &config.Config{}, t.Context())
			for _, kind := range []string{"smb", "local", "mismatch"} {
				target := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Draft", Secret: new("test-only")}
				if kind != "smb" {
					target = dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new(f.id), Path: "/srv/archive"}, Name: "Draft"}
				}
				if kind == "mismatch" {
					target.HostID = new("other")
				}
				creates := len(f.agentDB.creates)
				out, err := h.TestDraft(t.Context(), &struct{ Body dto.TargetDraftTest }{Body: dto.TargetDraftTest{Target: target, ProbeHostID: f.id}})
				if kind == "mismatch" {
					require.EqualError(t, err, "backup.target.probe-host-mismatch")
					require.Len(t, f.agentDB.creates, creates)
				} else {
					require.NoError(t, err)
					require.True(t, out.Body.Data.OK)
				}
			}
			for _, service := range []*Service{f.hub, f.agentDB.service} {
				var count int64
				require.NoError(t, service.deps.DB.Model(&backuptarget.Target{}).Count(&count).Error)
				require.Zero(t, count)
				require.NoError(t, service.deps.DB.Model(&backuptarget.Check{}).Count(&count).Error)
				require.Zero(t, count)
			}
		})
	}
}
