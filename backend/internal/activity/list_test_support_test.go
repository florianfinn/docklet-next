// Source: docklet-next/issues/108
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original activity list source-completeness contract and regression evidence.

package activity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

const activityListHost = "activity-remote"
const activityListReason = "X-Docklet-Host-Reason"

type activityListFixture struct {
	service      *ActivityService
	environments *environment.EnvironmentService
	handler      *ActivityHandler
	mux          *http.ServeMux
	api          huma.API
}

func newActivityListFixture(t *testing.T, isEdge bool, endpoint string, client *http.Client) *activityListFixture {
	t.Helper()
	db := setupActivityServiceTestDBInternal(t)
	require.NoError(t, db.Create(&environment.Environment{ID: activityListHost, Name: "Example host", ApiUrl: endpoint, Enabled: true, IsEdge: isEdge, AccessToken: new("example-agent-token")}).Error)
	environments := environment.NewEnvironmentService(db, client, nil, nil, nil, nil)
	service := NewActivityService(db, nil)
	h := NewHandler(service, EnvironmentDependencies{
		ProxyJSONRequest:       environments.ProxyJSONRequest,
		ResolveEnvironmentName: func(context.Context, string) string { return "Example host" },
	})
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Activity list test", "0.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	RegisterActivities(api, h)
	t.Cleanup(func() {
		edge.GetRegistry().Unregister(activityListHost)
		edge.ForgetPollState(activityListHost)
	})
	return &activityListFixture{service, environments, h, mux, api}
}

func (f *activityListFixture) request(t *testing.T, host, query string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/environments/"+host+"/activities"+query, nil))
	return rec
}

func (f *activityListFixture) seed(t *testing.T, host string) activity.Activity {
	t.Helper()
	run, err := f.service.StartActivity(t.Context(), StartActivityRequest{EnvironmentID: host, Type: activity.TypeImagePull, ResourceType: new("image"), ResourceID: new("example-image"), ResourceName: new("Example image")})
	require.NoError(t, err)
	_, err = f.service.AppendMessages(t.Context(), run.ID, []AppendActivityMessageRequest{{Message: "Pulling retained image", Code: "activity.message", Step: "pull"}})
	require.NoError(t, err)
	detail, err := f.service.GetActivityDetail(t.Context(), host, run.ID, 10)
	require.NoError(t, err)
	result := detail.Activity
	applyActivitySourceInternal(&result, host, "Example host")
	return result
}

func assertActivityListPage(t *testing.T, rec *httptest.ResponseRecorder, reason string, expected []activity.Activity) base.Paginated[activity.Activity] {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, reason, rec.Header().Get(activityListReason))
	if reason == "" {
		require.NotContains(t, rec.Header(), activityListReason)
	}
	require.Empty(t, rec.Header().Get("X-Docklet-Observed-At"))
	var page base.Paginated[activity.Activity]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.True(t, page.Success)
	if len(expected) == 0 {
		require.Empty(t, page.Data)
	} else {
		require.Equal(t, expected, page.Data)
	}
	for _, item := range page.Data {
		require.NotEmpty(t, item.SourceEnvironmentID)
		require.Equal(t, item.EnvironmentID, item.SourceEnvironmentID)
	}
	return page
}

func serveActivityAgent(t *testing.T, activities http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "example-agent-token", r.Header.Get("X-Arcane-Agent-Token"))
		switch r.URL.Path {
		case "/api/app-version":
			require.NoError(t, json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}))
		case "/api/environments/0/system/health":
			require.Equal(t, http.MethodHead, r.Method)
			w.WriteHeader(http.StatusOK)
		case "/api/environments/0/activities":
			activities(w, r)
		default:
			t.Errorf("unexpected agent path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeActivityAgentPage(t *testing.T, w http.ResponseWriter, rows []activity.Activity, total int64) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(base.Paginated[activity.Activity]{Success: true, Data: rows, Pagination: base.PaginationResponse{TotalItems: total}}))
}

func remoteActivityListRow() activity.Activity {
	return activity.Activity{ID: "agent-run", EnvironmentID: "0", Type: activity.TypeImagePull, Status: activity.StatusRunning, StartedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
}
