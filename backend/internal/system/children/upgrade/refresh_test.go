// Source: docklet-next/issues/439
// Revision: ea0f2bac35b56d205ca67ff6318d01d7270d301b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original proof that an explicit recheck refreshes the hub release list.

package upgrade

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/version"
	dto "github.com/florianfinn/docklet-next/types/system"
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
	check := func(refresh bool) dto.SelfUpdateEligibility {
		out, err := h.CheckUpgradeAvailable(t.Context(), &CheckUpgradeInput{EnvironmentID: f.remote.ID, Refresh: refresh})
		require.NoError(t, err)
		return out.Body.Eligibility
	}
	candidate := func(e dto.SelfUpdateEligibility) string {
		require.Equal(t, "available", e.State)
		require.NotNil(t, e.Candidate)
		return e.Candidate.Version
	}
	require.Equal(t, "1.0.1", candidate(check(false)))
	serve(`[`+release102+`,`+release101+`]`, http.StatusOK)
	require.Equal(t, "1.0.1", candidate(check(false)), "a plain check keeps the cached list")
	require.Equal(t, "1.0.2", candidate(check(true)))
}

func TestRecheckReportsAFailedRefresh(t *testing.T) {
	f, serve := refreshFixture(t)
	h := &Handler{service: f.hub}
	_, err := h.CheckUpgradeAvailable(t.Context(), &CheckUpgradeInput{EnvironmentID: f.remote.ID})
	require.NoError(t, err)
	serve(`{}`, http.StatusInternalServerError)
	out, err := h.CheckUpgradeAvailable(t.Context(), &CheckUpgradeInput{EnvironmentID: f.remote.ID, Refresh: true})
	require.NoError(t, err)
	require.Equal(t, []string{"unavailable", "update.check-unavailable"}, []string{out.Body.Eligibility.State, out.Body.Eligibility.Code})
	require.Nil(t, out.Body.Eligibility.Candidate)
	require.Equal(t, "1.0.0", out.Body.Version, "the installed version is still reported")
}
