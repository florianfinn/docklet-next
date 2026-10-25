// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: M10 controls stay on the hub; legacy mixed writes cannot partially change an agent.

package job

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLegacyM10ScheduleWritesRefuseBeforeForwardingOrPartialWrite(t *testing.T) {
	for _, agent := range []bool{false, true} {
		for _, body := range []string{
			`{"pollingInterval":"0 */10 * * * *","dockerClientRefreshInterval":"0 */2 * * * *"}`,
			`{"autoUpdateInterval":"0 0 8 * * *"}`,
			`{"pollingInterval":null}`,
			`{"autoUpdateInterval":null,"dockerClientRefreshInterval":"0 */2 * * * *"}`,
			`{"pollingInterval":{"invalid":"shape"}}`,
			`{"autoUpdateInterval":[1,2]}`,
		} {
			db := setupSettingsTestDBInternal(t)
			settings, err := newSettingsServiceForTestInternal(t, t.Context(), db)
			require.NoError(t, err)
			svc := newJobServiceForTestInternal(t, db, settings, &config.Config{AgentMode: agent})
			calls := 0
			h := &JobSchedulesHandler{jobService: svc, proxyRemoteJSON: func(context.Context, string, string, string, []byte, any) error {
				calls++
				return nil
			}}
			var update jobschedule.Update
			require.NoError(t, json.Unmarshal([]byte(body), &update))
			before := settings.GetStringSetting(t.Context(), "dockerClientRefreshInterval", "")
			_, err = h.Update(t.Context(), &UpdateJobSchedulesInput{ID: "example-remote", Body: update})
			require.Equal(t, "update.schedule-hub-owned", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
			require.Equal(t, 0, calls)
			require.Equal(t, before, settings.GetStringSetting(t.Context(), "dockerClientRefreshInterval", ""))
			_, err = svc.UpdateJobSchedules(t.Context(), update)
			require.Equal(t, "update.schedule-hub-owned", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
		}
	}
}
func TestLegacyScheduleHTTPNullIsTypedAndFieldsAreOmitted(t *testing.T) {
	db := setupSettingsTestDBInternal(t)
	settings, err := newSettingsServiceForTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	svc := newJobServiceForTestInternal(t, db, settings, &config.Config{})
	router := echo.New()
	api := humaecho.New(router, huma.DefaultConfig("test", "test"))
	RegisterJobSchedules(api, svc, nil)
	for _, body := range []string{`{"pollingInterval":null}`, `{"autoUpdateInterval":null}`, `{"pollingInterval":{"invalid":"shape"}}`, `{"autoUpdateInterval":[1,2]}`} {
		req := httptest.NewRequest(http.MethodPut, "/environments/0/job-schedules", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{Sudo: true}))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, 409, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), "update.schedule-hub-owned")
	}
	payload, err := json.Marshal(svc.GetJobSchedules(t.Context()))
	require.NoError(t, err)
	require.NotContains(t, string(payload), "autoUpdateInterval")
	require.NotContains(t, string(payload), "pollingInterval")
}
func TestAgentM10ControlsNeverReachForwarderOrCoordinator(t *testing.T) {
	svc := &JobService{cfg: &config.Config{AgentMode: true}}
	calls := 0
	h := &JobSchedulesHandler{jobService: svc, proxyRemoteJSON: func(context.Context, string, string, string, []byte, any) error {
		calls++
		return nil
	}}
	for _, kind := range []string{"image-polling", "auto-update"} {
		_, err := svc.Submit(t.Context(), st.Request{EnvironmentID: "example-remote", JobID: kind})
		require.Equal(t, "update.agent-local-job-disabled", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
		_, err = svc.RetryRun(t.Context(), "example-remote", kind, "example-run")
		require.Equal(t, "update.agent-local-job-disabled", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
		_, err = h.RestartWorker(t.Context(), &jobschedule.JobInput{ID: "example-remote", JobID: kind})
		require.Equal(t, "update.agent-local-job-disabled", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	}
	require.Zero(t, calls)
}
func TestPolicyRunProjectionUsesLogicalHostAndChildActivity(t *testing.T) {
	run := projectImagePolicyRun(st.Run{JobID: "image-policy:example-remote:auto-update", EnvironmentID: "0", ActivityID: "example-activity", Outcome: st.Outcome{Targets: []st.TargetOutcome{{ID: "stack", ActivityID: "example-child", Code: "update.config-drift", Status: st.Skipped}}}})
	require.Equal(t, "example-remote", run.EnvironmentID)
	require.Equal(t, "example-remote", run.ActivityEnvironmentID)
	require.Equal(t, "auto-update", run.JobID)
	require.Equal(t, "example-child", run.Outcome.Targets[0].ActivityID)
	require.Equal(t, "update.config-drift", run.Outcome.Targets[0].Code)
}
