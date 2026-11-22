// Source: docklet-next/issues/571
// Revision: 401bacecd34950fed3530fa414eb289f19833146
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original stored-target repository check through hub delivery, agent worker and hub relay.

package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

const (
	remoteAgentID    = "env-7"
	remoteAgentToken = "test-only"
)

func sudoContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
	return context.WithValue(ctx, user.CurrentUserContextKey{}, &user.SystemUser)
}

// remoteCheck runs the hub's remote target test sequence (TargetHandler.Test) against a real agent router
// reached over Direct or Edge, waits for the agent worker, collects the result on the hub and reads the
// activity through the hub activity handler that relays agent activities.
func remoteCheck(t *testing.T, mode, kind string, arrange func(*runtimePlan)) (backuptarget.Check, *activitytypes.Detail) {
	t.Helper()
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	agent := newRuntimePlan(t)
	arrange(agent)
	require.NoError(t, agent.service.deps.DB.AutoMigrate(append(activity.Models(), &backuptarget.Target{}, &backuptarget.Check{})...))
	agent.service.deps.Activity = activity.NewActivityService(agent.service.deps.DB, nil)
	router, root := humatest.New(t)
	api := huma.NewGroup(root, "/api")
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, sudoContext(ctx.Context())))
	})
	backuptarget.RegisterHostRoutes(api, TargetHandler(agent.service, nil, &config.Config{AgentMode: true, AgentToken: remoteAgentToken}, t.Context()), true)
	activity.RegisterActivities(api, activity.NewHandler(agent.service.deps.Activity, activity.EnvironmentDependencies{}))
	// The hub's authenticated agent access presents the pairing token on every request.
	call := backupRouteTransport(t, mode, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Arcane-Agent-Token", remoteAgentToken)
		router.ServeHTTP(w, r)
	}))
	proxy := func(ctx context.Context, host, method, path string, body []byte, out any) error {
		require.Equal(t, remoteAgentID, host)
		status, _, payload := call(ctx, method, path, body)
		if status >= 300 {
			return &remenv.StatusError{StatusCode: status, Body: payload}
		}
		return json.Unmarshal(payload, out)
	}

	hub := targetAdmissionService(t)
	require.NoError(t, hub.deps.DB.AutoMigrate(activity.Models()...))
	ctx := sudoContext(t.Context())
	host := remoteAgentID
	target, err := hub.targets.Create(ctx, dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: kind, HostID: &host, Path: "/srv/backups"}, Name: "Archive"})
	require.NoError(t, err)
	receipt, err := backuptarget.SyncHost(ctx, hub.deps.DB, remoteAgentID, proxy)
	require.NoError(t, err)
	response, err := backuptarget.RemoteJSON[base.ApiResponse[dto.TargetTestOperation]](ctx, proxy, remoteAgentID, http.MethodPost, "/api/environments/0/volumes/backups/targets/"+target.ID+"/test", nil)
	require.NoError(t, err)
	activityID := response.Data.ActivityID
	require.NoError(t, hub.recordRemoteCheck(ctx, target.ID, remoteAgentID, receipt.SetVersion, activityID))
	require.Eventually(t, func() bool {
		detail, err := agent.service.deps.Activity.GetActivityDetail(ctx, "0", activityID, 50)
		return err == nil && detail.Activity.Status != activitytypes.StatusRunning && detail.Activity.Status != activitytypes.StatusQueued
	}, 20*time.Second, 20*time.Millisecond)

	require.NoError(t, hub.refreshTargetChecks(ctx, proxy))
	var saved backuptarget.Check
	require.NoError(t, hub.deps.DB.First(&saved, "target_id = ? AND host_id = ?", target.ID, remoteAgentID).Error)
	relay := activity.NewHandler(activity.NewActivityService(hub.deps.DB, nil), activity.EnvironmentDependencies{ProxyJSONRequest: proxy, ResolveEnvironmentName: func(context.Context, string) string { return "Agent" }})
	out, err := relay.GetActivity(ctx, &activity.GetActivityInput{EnvironmentID: remoteAgentID, ActivityID: activityID, Limit: 50})
	require.NoError(t, err)
	return saved, &out.Body.Data
}

func TestRepositoryCheckStoredTargetFailsAtConnectOnTheAgent(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			saved, detail := remoteCheck(t, mode, "mount", func(f *runtimePlan) { f.witness = "1\n1" })
			require.Equal(t, activitytypes.StatusFailed, detail.Activity.Status)
			steps, texts := traceAndOutput(detail)
			require.Equal(t, []string{"connect"}, steps)
			require.Equal(t, []string{repositoryCheckStages["connect"]}, texts)
			require.False(t, saved.OK)
			require.Equal(t, "backup.target.not-mounted", saved.Code, "the hub keeps the agent's problem code")
			require.Nil(t, saved.SnapshotCount)
			check, _ := detail.Activity.Metadata["backupTargetCheck"].(map[string]any)
			require.Equal(t, "backup.target.not-mounted", check["code"])
			require.Nil(t, check["snapshotCount"])
		})
	}
}

// The relayed stored-target detail is the web reader's fixture; both transports deliver the same bytes.
func TestRepositoryCheckStoredTargetResultReachesTheHubAndWebFixture(t *testing.T) {
	var fixtures [][]byte
	for _, mode := range []string{"direct", "edge"} {
		saved, detail := remoteCheck(t, mode, "local", func(f *runtimePlan) {
			f.service.deps.Exec = func(context.Context, string, string, []string) (string, string, error) { return "present", "", nil }
			f.stdout = `[{"id":"` + strings.Repeat("a", 64) + `"},{"id":"` + strings.Repeat("b", 64) + `"}]`
		})
		require.Equal(t, activitytypes.StatusSuccess, detail.Activity.Status, mode)
		require.True(t, saved.OK, mode)
		require.NotNil(t, saved.SnapshotCount)
		require.EqualValues(t, 2, *saved.SnapshotCount, "the hub collector stores the agent's count")
		require.Equal(t, remoteAgentID, detail.Activity.EnvironmentID, "the hub relays the agent activity under its registration")
		raw, err := json.Marshal(detail)
		require.NoError(t, err)
		raw = bytes.ReplaceAll(raw, []byte(detail.Activity.ID), []byte("check-run"))
		raw = bytes.ReplaceAll(raw, []byte(saved.TargetID), []byte("archive"))
		raw = producerTimestamp.ReplaceAll(raw, []byte(`"2026-11-22T10:00:00Z"`))
		raw = producerDuration.ReplaceAll(raw, []byte(`"durationMs":0`))
		ids := map[string]string{}
		raw = producerID.ReplaceAllFunc(raw, func(id []byte) []byte {
			if _, ok := ids[string(id)]; !ok {
				ids[string(id)] = fmt.Sprintf("stored-m%d", len(ids)+1)
			}
			return []byte(ids[string(id)])
		})
		var shape any
		require.NoError(t, json.Unmarshal(raw, &shape))
		body, err := json.MarshalIndent(shape, "", "  ")
		require.NoError(t, err)
		fixtures = append(fixtures, append(body, '\n'))
	}
	require.Equal(t, string(fixtures[0]), string(fixtures[1]), "Direct and Edge relay the same detail")
	path := "../../../../../web/src/data/http/testdata/repository-check-stored.json"
	if os.Getenv("UPDATE_WEB_FIXTURES") == "1" {
		require.NoError(t, os.WriteFile(path, fixtures[0], 0644))
	}
	expected, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(expected, fixtures[0]), "web fixture differs from producer bytes; regenerate with UPDATE_WEB_FIXTURES=1")
}
