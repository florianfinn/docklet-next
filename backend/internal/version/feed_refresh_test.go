// Source: docklet-next/issues/235
// Revision: ea0f2bac35b56d205ca67ff6318d01d7270d301b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original tests for the explicit release feed refresh.

package version

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

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

	svc.feed.forcedAt = svc.feed.forcedAt.Add(-releaseFeedMinRefresh)
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

// gatedFeedInternal holds the first request until release is closed. Each
// request answers with the upstream state at its start.
type gatedFeedInternal struct {
	mu       sync.Mutex
	body     string
	fail     bool
	requests int
	started  chan struct{}
	release  chan struct{}
}

func (g *gatedFeedInternal) set(body string, fail bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.body, g.fail = body, fail
}

func (g *gatedFeedInternal) client() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		g.mu.Lock()
		n, body, fail := g.requests, g.body, g.fail
		g.requests++
		g.mu.Unlock()
		if n == 0 {
			close(g.started)
			<-g.release
		}
		if fail {
			return nil, errors.New("dial failed")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
}

func TestReleaseFeedOlderLoadCannotReplaceARefresh(t *testing.T) {
	for _, tc := range []struct {
		name      string
		stale     bool
		olderFail bool
	}{{"cold load", false, false}, {"stale reload", true, false}, {"older load fails", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			gate := &gatedFeedInternal{body: `[` + feedEntryJSON("v1.2.0") + `]`, started: make(chan struct{}), release: make(chan struct{})}
			svc := NewVersionService(gate.client(), false, "", "1.2.0", "", nil, nil)
			if tc.stale {
				svc.feed.list, svc.feed.listAt = nil, time.Now().Add(-releaseFeedTTL)
			}
			gate.set(`[`+feedEntryJSON("v1.2.0")+`]`, tc.olderFail)
			older := make(chan struct{})
			go func() { defer close(older); _, _, _ = svc.NewestRelease(context.Background()) }()
			<-gate.started

			gate.set(`[`+feedEntryJSON("v1.3.0")+`,`+feedEntryJSON("v1.2.0")+`]`, false)
			refreshed := make(chan error, 1)
			go func() { refreshed <- svc.RefreshReleases(context.Background()) }()
			// The refresh must not queue behind the older fetch (browser deadline).
			select {
			case err := <-refreshed:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				close(gate.release)
				t.Fatal("the refresh waited for the older fetch")
			}
			newest, _, err := svc.NewestRelease(t.Context())
			require.NoError(t, err)
			require.Equal(t, "1.3.0", newest.Version, "the refresh is visible while the older fetch still runs")
			close(gate.release)
			<-older

			newest, _, err = svc.NewestRelease(t.Context())
			require.NoError(t, err)
			require.Equal(t, "1.3.0", newest.Version, "the older fetch finishing last does not undo the refresh")
		})
	}
}

func TestReleaseFeedWaitsBeforeRetryingAFailedFetch(t *testing.T) {
	feed := &switchableFeedInternal{fail: true}
	svc := NewVersionService(feed.client(), false, "", "1.2.0", "", nil, nil)
	_, _, err := svc.NewestRelease(t.Context())
	require.Equal(t, "update.check-unavailable", codeOf(err))
	_, _, err = svc.NewestRelease(t.Context())
	require.Equal(t, "update.check-unavailable", codeOf(err))
	require.Equal(t, 1, feed.count(), "a failed fetch is not repeated by every read")

	feed.set(`[`+feedEntryJSON("v1.2.0")+`]`, false)
	svc.feed.failedAt = svc.feed.failedAt.Add(-releaseFeedRetry)
	newest, _, err := svc.NewestRelease(t.Context())
	require.NoError(t, err)
	require.Equal(t, "1.2.0", newest.Version)
	require.Equal(t, 2, feed.count())
}

func TestReleaseFeedConcurrentReadsShareOneFetch(t *testing.T) {
	gate := &gatedFeedInternal{body: `[` + feedEntryJSON("v1.2.0") + `]`, started: make(chan struct{}), release: make(chan struct{})}
	svc := NewVersionService(gate.client(), false, "", "1.2.0", "", nil, nil)
	var wg sync.WaitGroup
	wg.Go(func() { _, _, _ = svc.NewestRelease(context.Background()) })
	<-gate.started
	for range 10 {
		wg.Go(func() {
			newest, _, err := svc.NewestRelease(context.Background())
			if err != nil || newest.Version != "1.2.0" {
				t.Errorf("reader got %q, %v", newest.Version, err)
			}
		})
	}
	close(gate.release)
	wg.Wait()
	gate.mu.Lock()
	defer gate.mu.Unlock()
	require.Equal(t, 1, gate.requests)
}
