// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Actual hub schedule storage and logical controls remain usable with an offline agent.

package job

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type policyOfflineTransport struct{ calls int }

func (p *policyOfflineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	p.calls++
	return nil, errors.New("synthetic offline agent")
}
func TestImagePolicyOfflineSchedulesUseHubZoneAndControls(t *testing.T) {
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	settings, err := newSettingsServiceForTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	cfg := &config.Config{Timezone: "Europe/Berlin"}
	events := event.NewEventService(db, cfg, nil)
	activities := activity.NewActivityService(db, settings)
	transport := &policyOfflineTransport{}
	envs := environment.NewEnvironmentService(db, &http.Client{Transport: transport}, nil, events, settings, nil)
	require.NoError(t, db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: "example-offline"}, Name: "Example offline", ApiUrl: "https://agent.example.com", Enabled: true, Status: "offline"}).Error)
	require.NoError(t, db.Create(&userdomain.User{BaseModel: database.BaseModel{ID: "example-actor"}, Username: "operator"}).Error)
	require.NoError(t, db.Create(&role.Role{ID: "example-jobs-role", Name: "Jobs manager", Permissions: database.StringSlice{authz.PermJobsManage}}).Error)
	require.NoError(t, db.Create(&role.UserRoleAssignment{UserID: "example-actor", RoleID: "example-jobs-role"}).Error)
	roles := role.NewRoleService(db)
	owner, err := updater.NewUpdaterService(db, settings, nil, nil, nil, nil, events, nil, nil, nil, activities, cfg, nil, nil, roles)
	require.NoError(t, err)
	refreshes := 0
	owner.ConfigureImagePolicies(envs, func(context.Context) error {
		refreshes++
		return nil
	})
	q := newJobCoordinatorForTestInternal(t, db)
	svc := NewJobService(db, settings, cfg, q, roles, envs, activities)
	svc.SetImagePolicies(owner)
	before, err := svc.ImagePolicySchedules(t.Context(), "example-offline")
	require.NoError(t, err)
	require.Equal(t, "Europe/Berlin", before.TimeZone)
	require.Len(t, before.Jobs, 2)
	require.False(t, before.Jobs[0].Enabled)
	for _, job := range before.Jobs {
		require.Equal(t, "update.schedule-owner-unset", job.Code)
	}
	require.Nil(t, before.Jobs[0].NextRun)
	input := dto.UpdateScheduleInput{PollingInterval: "0  30  2 * * *", AutoUpdateInterval: "0 0 0 * * *", PollingEnabled: true, AutoUpdateEnabled: true}
	require.NoError(t, owner.SaveImagePolicySchedule(t.Context(), "example-offline", input, "example-actor", ""))
	require.Equal(t, 1, refreshes)
	require.Error(t, owner.SaveImagePolicySchedule(t.Context(), "example-offline", input, "example-actor", ""), "revision CAS prevents lost edits")
	after, err := svc.ImagePolicySchedules(t.Context(), "example-offline")
	require.NoError(t, err)
	require.EqualValues(t, 1, after.Revision)
	require.Equal(t, "0 30 2 * * *", after.PollingInterval)
	router := echo.New()
	api := humaecho.New(router, huma.DefaultConfig("test", "test"))
	registerImagePolicyScheduleRoutes(api, svc)
	input.ExpectedRevision = 1
	body, err := json.Marshal(input)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut, "/environments/example-offline/updater/schedules", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	contextWithActor := context.WithValue(request.Context(), middleware.ContextKeyUserID, "example-actor")
	contextWithActor = context.WithValue(contextWithActor, middleware.ContextKeyUserPermissions, &authz.PermissionSet{Sudo: true})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request.WithContext(contextWithActor))
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"revision":2`)
	parsed, err := schedule.Parser().Parse(after.PollingInterval)
	require.NoError(t, err)
	zone, err := time.LoadLocation(after.TimeZone)
	require.NoError(t, err)
	spring := parsed.Next(time.Date(2026, 3, 29, 1, 59, 59, 0, zone))
	require.Equal(t, 30, spring.Day())
	require.Equal(t, 2, spring.Hour())
	autumn := parsed.Next(time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC).In(zone))
	require.Equal(t, 25, autumn.Day())
	require.Equal(t, 2, autumn.Hour(), "repeated DST time uses the hub parser and zone")
	require.Zero(t, transport.calls, "dedicated reads/writes do not probe an offline agent")
	run, err := svc.Submit(t.Context(), st.Request{JobID: "image-polling", EnvironmentID: "example-offline", RequestedBy: "example-actor", Trigger: "manual"})
	require.NoError(t, err)
	require.Equal(t, "image-policy:example-offline:image-polling", run.JobID)
	require.Equal(t, "0", run.EnvironmentID)
	h := &JobSchedulesHandler{jobService: svc}
	out, err := h.GetRun(t.Context(), &jobschedule.RunInput{ID: "example-offline", JobID: "image-polling", RunID: run.ID})
	require.NoError(t, err)
	require.Equal(t, "example-offline", out.Body.EnvironmentID)
	require.Equal(t, "image-polling", out.Body.JobID)
	_, err = svc.CancelRun(t.Context(), "example-offline", "image-polling", run.ID)
	require.NoError(t, err)
	require.Zero(t, transport.calls)
	require.NoError(t, db.Where("user_id = ?", "example-actor").Delete(&role.UserRoleAssignment{}).Error)
	input.ExpectedRevision = 2
	require.Error(t, owner.SaveImagePolicySchedule(t.Context(), "example-offline", input, "example-actor", ""), "schedule writes resolve current grants")
}
