// Source: docklet-next/issues/439
// Revision: ea0f2bac35b56d205ca67ff6318d01d7270d301b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original proof that the hub-only refresh renews the release list for every host.

package upgrade

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/stretchr/testify/require"
)

const (
	release101 = `{"tag_name":"v1.0.1","published_at":"2026-11-01T12:00:00Z"}`
	release102 = `{"tag_name":"v1.0.2","published_at":"2026-11-02T12:00:00Z"}`
)

// refreshFixture serves a release list the test replaces; the hub runs 1.0.2
// so the remote agent (1.0.0) is offered the newest listed release up to it.
func refreshFixture(t *testing.T) (*updateTransportFixture, func(body string, status int)) {
	t.Helper()
	updateStamp(t)
	f := newUpdateTransport(t, false)
	var mu sync.Mutex
	body, status := `[`+release101+`]`, http.StatusOK
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(feed.Close)
	f.hub.versions = version.NewVersionService(feed.Client(), false, feed.URL, "1.0.2", "example", nil, nil)
	f.hub.runningVersion = func() string { return "1.0.2" }
	return f, func(next string, code int) { mu.Lock(); defer mu.Unlock(); body, status = next, code }
}

func TestRecheckRefreshesTheHubReleaseList(t *testing.T) {
	f, serve := refreshFixture(t)
	h := &Handler{service: f.hub}
	candidate := func() string {
		out, err := h.CheckUpgradeAvailable(t.Context(), &CheckUpgradeInput{EnvironmentID: f.remote.ID})
		require.NoError(t, err)
		require.Equal(t, "available", out.Body.Eligibility.State)
		require.NotNil(t, out.Body.Eligibility.Candidate)
		return out.Body.Eligibility.Candidate.Version
	}
	require.Equal(t, "1.0.1", candidate())
	serve(`[`+release102+`,`+release101+`]`, http.StatusOK)
	require.Equal(t, "1.0.1", candidate(), "a plain check keeps the cached list")
	_, err := h.RefreshReleases(t.Context(), &struct{}{})
	require.NoError(t, err)
	require.Equal(t, "1.0.2", candidate())
}

func TestRecheckReportsAFailedRefresh(t *testing.T) {
	f, serve := refreshFixture(t)
	h := &Handler{service: f.hub}
	serve(`{}`, http.StatusInternalServerError)
	_, err := h.RefreshReleases(t.Context(), &struct{}{})
	var problem huma.StatusError
	require.ErrorAs(t, err, &problem)
	require.Equal(t, http.StatusServiceUnavailable, problem.GetStatus())
	require.Contains(t, err.Error(), "update.check-unavailable")
}

func TestRefreshRouteIsHubOnly(t *testing.T) {
	for name, register := range map[string]func(huma.API, *Handler){"hub": RegisterRoutes, "agent": RegisterAgentRoutes} {
		_, api := humatest.New(t)
		register(api, &Handler{})
		_, found := api.OpenAPI().Paths["/system/releases/refresh"]
		require.Equal(t, name == "hub", found, name)
	}
}
