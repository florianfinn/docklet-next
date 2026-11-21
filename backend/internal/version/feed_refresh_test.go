// Source: docklet-next/issues/235
// Revision: ea0f2bac35b56d205ca67ff6318d01d7270d301b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original tests for the explicit release feed refresh.

package version

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// switchableFeedInternal serves one page whose body or failure the test changes.
type switchableFeedInternal struct {
	mu       sync.Mutex
	body     string
	fail     bool
	requests int
}

func (s *switchableFeedInternal) set(body string, fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body, s.fail = body, fail
}

func (s *switchableFeedInternal) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

func (s *switchableFeedInternal) client() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests++
		if s.fail {
			return nil, errors.New("dial failed")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(s.body)), Header: http.Header{}, Request: r}, nil
	})}
}

func TestReleaseFeedRefreshReplacesTheCachedList(t *testing.T) {
	feed := &switchableFeedInternal{body: `[` + feedEntryJSON("v1.2.0") + `]`}
	svc := NewVersionService(feed.client(), false, "", "1.2.0", "", nil, nil)
	newest, _, err := svc.NewestRelease(t.Context())
	require.NoError(t, err)
	require.Equal(t, "1.2.0", newest.Version)

	feed.set(`[`+feedEntryJSON("v1.3.0")+`,`+feedEntryJSON("v1.2.0")+`]`, false)
	newest, _, _ = svc.NewestRelease(t.Context())
	require.Equal(t, "1.2.0", newest.Version, "a plain read keeps the cached list")

	require.NoError(t, svc.RefreshReleases(t.Context()))
	newest, _, err = svc.NewestRelease(t.Context())
	require.NoError(t, err)
	require.Equal(t, "1.3.0", newest.Version)
}

func TestReleaseFeedRefreshIsThrottled(t *testing.T) {
	feed := &switchableFeedInternal{body: `[` + feedEntryJSON("v1.2.0") + `]`}
	svc := NewVersionService(feed.client(), false, "", "1.2.0", "", nil, nil)
	require.NoError(t, svc.RefreshReleases(t.Context()))
	require.Equal(t, 1, feed.count())
	require.NoError(t, svc.RefreshReleases(t.Context()))
	require.NoError(t, svc.RefreshReleases(t.Context()))
	require.Equal(t, 1, feed.count(), "refreshes inside the window make no request")

	svc.feed.refreshed = svc.feed.refreshed.Add(-releaseFeedMinRefresh)
	require.NoError(t, svc.RefreshReleases(t.Context()))
	require.Equal(t, 2, feed.count())
}

func TestReleaseFeedRefreshFailureIsReportedAndKeepsTheList(t *testing.T) {
	feed := &switchableFeedInternal{body: `[` + feedEntryJSON("v1.2.0") + `]`}
	svc := NewVersionService(feed.client(), false, "", "1.2.0", "", nil, nil)
	_, _, err := svc.NewestRelease(t.Context())
	require.NoError(t, err)

	feed.set("", true)
	require.Equal(t, "update.check-unavailable", codeOf(svc.RefreshReleases(t.Context())))
	requests := feed.count()
	require.Equal(t, "update.check-unavailable", codeOf(svc.RefreshReleases(t.Context())), "the window repeats the last result")
	require.Equal(t, requests, feed.count())
	newest, _, err := svc.NewestRelease(t.Context())
	require.NoError(t, err)
	require.Equal(t, "1.2.0", newest.Version)
}

func TestReleaseFeedRefreshDisabledMakesNoRequest(t *testing.T) {
	feed := &switchableFeedInternal{body: `[` + feedEntryJSON("v1.3.0") + `]`}
	svc := NewVersionService(feed.client(), true, "", "1.2.0", "", nil, nil)
	require.NoError(t, svc.RefreshReleases(t.Context()))
	require.Zero(t, feed.count())
}
