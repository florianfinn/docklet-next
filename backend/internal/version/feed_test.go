// Source: docklet-next/issues/235
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original release feed tests over a recording transport.

package version

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
)

type recordingFeedInternal struct {
	mu       sync.Mutex
	requests []string
	client   *http.Client
}

func (r *recordingFeedInternal) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

// newRecordingFeedInternal answers page N with pages[N-1] (status 200) and
// records host+path+query of every request.
func newRecordingFeedInternal(t *testing.T, pages ...string) *recordingFeedInternal {
	t.Helper()
	return newStatusFeedInternal(t, http.StatusOK, pages...)
}

func newStatusFeedInternal(t *testing.T, status int, pages ...string) *recordingFeedInternal {
	t.Helper()
	feed := &recordingFeedInternal{}
	feed.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		feed.mu.Lock()
		feed.requests = append(feed.requests, r.URL.Scheme+"://"+r.URL.Host+r.URL.Path+"?"+r.URL.RawQuery)
		feed.mu.Unlock()
		body := "[]"
		page := 0
		_, _ = fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		if page >= 1 && page <= len(pages) {
			body = pages[page-1]
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	return feed
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func feedEntryJSON(tag string) string {
	return `{"tag_name":"` + tag + `","published_at":"2026-11-01T00:00:00Z","html_url":"https://github.com/florianfinn/docklet-next/releases/tag/` + tag + `","body":"notes ` + tag + `"}`
}

func codeOf(err error) string { return failureCodeInternal(err) }

func TestReleaseFeedDefaultContactsOnlyTheProductRepository(t *testing.T) {
	feed := newRecordingFeedInternal(t, `[`+feedEntryJSON("v1.0.0")+`,`+feedEntryJSON("v1.1.0")+`]`)
	svc := NewVersionService(feed.client, false, "", "1.0.0", "", nil, nil)

	info := svc.GetAppVersionInfo(t.Context())
	require.True(t, info.UpdateAvailable)
	require.Equal(t, "v1.1.0", info.NewestVersion)
	require.Equal(t, "https://github.com/florianfinn/docklet-next/releases/tag/v1.1.0", info.ReleaseURL)
	require.Equal(t, "notes v1.1.0", info.ReleaseNotes)

	check, err := svc.GetVersionInformation(t.Context(), "")
	require.NoError(t, err)
	require.True(t, check.UpdateAvailable)
	require.Equal(t, info.ReleaseURL, check.ReleaseURL)

	require.Equal(t, []string{"https://api.github.com/repos/florianfinn/docklet-next/releases?page=1&per_page=100"}, feed.seen(), "one cached fetch, only the product repository")
}

func TestReleaseFeedKeepsOnlyPublishedStableVTags(t *testing.T) {
	entries := []string{
		feedEntryJSON("v1.2.0"),
		`{"tag_name":"v9.0.0","draft":true,"published_at":"2026-11-01T00:00:00Z"}`,
		`{"tag_name":"v8.0.0","prerelease":true,"published_at":"2026-11-01T00:00:00Z"}`,
		`{"tag_name":"v7.0.0-rc.1","published_at":"2026-11-01T00:00:00Z"}`,
		`{"tag_name":"6.0.0","published_at":"2026-11-01T00:00:00Z"}`,
		`{"tag_name":"v5.0.0"}`,
		`{"tag_name":"v04.0.0","published_at":"2026-11-01T00:00:00Z"}`,
		`{"tag_name":"v3.0.0+build.1","published_at":"2026-11-01T00:00:00Z"}`,
		`{"tag_name":"v2.0.0","published_at":"2026-11-01T00:00:00Z","html_url":"javascript:alert(1)"}`,
	}
	svc := NewVersionService(newRecordingFeedInternal(t, "["+strings.Join(entries, ",")+"]").client, false, "", "1.0.0", "", nil, nil)
	newest, ok, err := svc.NewestRelease(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "2.0.0", newest.Version)
	require.Empty(t, newest.URL, "a non-http link from the feed is dropped")
	capped, ok, err := svc.ReleaseAtMost(t.Context(), "1.5.0")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "1.2.0", capped.Version)
}

func TestReleaseFeedEmptyOr404MeansNoReleasePublished(t *testing.T) {
	for name, feed := range map[string]*recordingFeedInternal{
		"empty": newRecordingFeedInternal(t, "[]"),
		"404":   newStatusFeedInternal(t, http.StatusNotFound, `{"message":"Not Found"}`),
	} {
		t.Run(name, func(t *testing.T) {
			svc := NewVersionService(feed.client, false, "", "1.0.0", "", nil, nil)
			_, ok, err := svc.NewestRelease(t.Context())
			require.NoError(t, err)
			require.False(t, ok)
			_, ok, err = svc.ReleaseAtMost(t.Context(), "1.0.0")
			require.NoError(t, err)
			require.False(t, ok)
			info := svc.GetAppVersionInfo(t.Context())
			require.False(t, info.UpdateAvailable)
			require.Empty(t, info.NewestVersion)
			check, err := svc.GetVersionInformation(t.Context(), "")
			require.NoError(t, err)
			require.False(t, check.UpdateAvailable)
		})
	}
}

func TestReleaseFeedFailuresAreUnavailable(t *testing.T) {
	failing := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})}
	cases := map[string]*http.Client{
		"500":       newStatusFeedInternal(t, http.StatusInternalServerError, `{}`).client,
		"transport": failing,
		"bad json":  newRecordingFeedInternal(t, `{"not":"a list"}`).client,
		"oversized": newRecordingFeedInternal(t, `[`+strings.Repeat(" ", releaseFeedPageBytes)+`]`).client,
	}
	for name, client := range cases {
		t.Run(name, func(t *testing.T) {
			svc := NewVersionService(client, false, "", "1.0.0", "", nil, nil)
			_, _, err := svc.NewestRelease(t.Context())
			require.Equal(t, "update.check-unavailable", codeOf(err))
			_, _, err = svc.ReleaseAtMost(t.Context(), "1.0.0")
			require.Equal(t, "update.check-unavailable", codeOf(err))
			_, err = svc.GetVersionInformation(t.Context(), "")
			require.Error(t, err)
			require.False(t, svc.GetAppVersionInfo(t.Context()).UpdateAvailable)
		})
	}
}

func TestReleaseFeedDisabledMakesNoRequest(t *testing.T) {
	feed := newRecordingFeedInternal(t, `[`+feedEntryJSON("v2.0.0")+`]`)
	svc := NewVersionService(feed.client, true, "", "1.0.0", "", nil, nil)
	require.False(t, svc.GetAppVersionInfo(t.Context()).UpdateAvailable)
	check, err := svc.GetVersionInformation(t.Context(), "")
	require.NoError(t, err)
	require.False(t, check.UpdateAvailable)
	_, _, err = svc.NewestRelease(t.Context())
	require.Equal(t, "feature-disabled", codeOf(err))
	_, _, err = svc.ReleaseAtMost(t.Context(), "1.0.0")
	require.Equal(t, "feature-disabled", codeOf(err))
	_, err = svc.PublishedReleaseAtMost(t.Context(), "1.0.0")
	require.Equal(t, "update.requires-hub-update", codeOf(err))
	require.Empty(t, feed.seen())
}

func TestReleaseFeedDevBuildMakesNoRequest(t *testing.T) {
	feed := newRecordingFeedInternal(t, `[`+feedEntryJSON("v2.0.0")+`]`)
	svc := NewVersionService(feed.client, false, "", "dev", "", nil, nil)
	require.False(t, svc.GetAppVersionInfo(t.Context()).UpdateAvailable)
	_, err := svc.GetVersionInformation(t.Context(), "")
	require.NoError(t, err)
	_, _, err = svc.ReleaseAtMost(t.Context(), "dev")
	require.Equal(t, "update.system-version-unknown", codeOf(err))
	require.Empty(t, feed.seen())
}

func TestReleaseFeedOverrideIsUsedByEveryConsumer(t *testing.T) {
	feed := newRecordingFeedInternal(t, `[`+feedEntryJSON("v1.0.0")+`]`)
	svc := NewVersionService(feed.client, false, "https://mirror.example.com/releases", "1.0.0", "", nil, nil)
	_ = svc.GetAppVersionInfo(t.Context())
	_, _ = svc.GetVersionInformation(t.Context(), "")
	_, _, _ = svc.ReleaseAtMost(t.Context(), "1.0.0")
	_, _ = svc.PublishedReleaseAtMost(t.Context(), "1.0.0")
	for _, request := range feed.seen() {
		require.True(t, strings.HasPrefix(request, "https://mirror.example.com/releases?"), request)
	}
	require.NotEmpty(t, feed.seen())

	// A new service (restart) with another URL never reuses the earlier cache.
	other := newRecordingFeedInternal(t, `[`+feedEntryJSON("v1.0.0")+`]`)
	_, _, err := NewVersionService(other.client, false, "https://feed.example.org/list", "1.0.0", "", nil, nil).NewestRelease(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"https://feed.example.org/list?page=1&per_page=100"}, other.seen())
}

func TestReleaseFeedCleartextOnlyForLocalHosts(t *testing.T) {
	resolver := func(addrs ...string) func(context.Context, string) ([]netip.Addr, error) {
		return func(context.Context, string) ([]netip.Addr, error) {
			if len(addrs) == 0 {
				return nil, errors.New("no such host")
			}
			out := make([]netip.Addr, 0, len(addrs))
			for _, a := range addrs {
				out = append(out, netip.MustParseAddr(a))
			}
			return out, nil
		}
	}
	cases := []struct {
		name, url string
		lookup    func(context.Context, string) ([]netip.Addr, error)
		allowed   bool
	}{
		{"https public", "https://feed.example.com/releases", nil, true},
		{"loopback v4", "http://127.0.0.1:8080/releases", nil, true},
		{"loopback v6", "http://[::1]:8080/releases", nil, true},
		{"rfc1918", "http://10.1.2.3/releases", nil, true},
		{"ula", "http://[fd00::1]/releases", nil, true},
		{"link-local", "http://[fe80::1]/releases", nil, true},
		{"public literal", "http://192.0.2.10/releases", nil, false},
		{"public v6 literal", "http://[2001:db8::1]/releases", nil, false},
		{"name to private", "http://feed.internal.example/releases", resolver("192.168.1.5", "fd00::5"), true},
		{"name to mixed", "http://feed.internal.example/releases", resolver("192.168.1.5", "192.0.2.5"), false},
		{"name unresolved", "http://feed.internal.example/releases", resolver(), false},
		{"ftp scheme", "ftp://feed.example.com/releases", nil, false},
		{"credentials", "https://user:pass@feed.example.com/releases", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			feed := newRecordingFeedInternal(t, `[`+feedEntryJSON("v1.0.0")+`]`)
			svc := NewVersionService(feed.client, false, tc.url, "1.0.0", "", nil, nil)
			if tc.lookup != nil {
				svc.feed.lookup = tc.lookup
			}
			_, ok, err := svc.NewestRelease(t.Context())
			if tc.allowed {
				require.NoError(t, err)
				require.True(t, ok)
				require.Len(t, feed.seen(), 1)
				return
			}
			require.Equal(t, "update.check-unavailable", codeOf(err))
			require.Empty(t, feed.seen(), "refused feed URLs are never requested")
		})
	}
}

func TestReleaseAtMostOrdersReleaseCandidates(t *testing.T) {
	svc := NewVersionService(newRecordingFeedInternal(t, `[`+feedEntryJSON("v1.0.0")+`,`+feedEntryJSON("v0.9.0")+`]`).client, false, "", "1.0.0-rc.2", "", nil, nil)
	capped, ok, err := svc.ReleaseAtMost(t.Context(), "1.0.0-rc.2")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0.9.0", capped.Version)
	capped, ok, err = svc.ReleaseAtMost(t.Context(), "1.0.0")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "1.0.0", capped.Version)
	info := svc.GetAppVersionInfo(t.Context())
	require.True(t, info.UpdateAvailable, "an rc hub sees the final release as newer")
	require.Equal(t, "v1.0.0", info.NewestVersion)
}

func TestReleaseFeedPagingStopsAtShortPageAndRefusesOverlongFeeds(t *testing.T) {
	full := "[" + strings.TrimSuffix(strings.Repeat(feedEntryJSON("v0.1.0")+",", releaseFeedPageSize), ",") + "]"
	feed := newRecordingFeedInternal(t, full, `[`+feedEntryJSON("v0.2.0")+`]`)
	newest, ok, err := NewVersionService(feed.client, false, "", "1.0.0", "", nil, nil).NewestRelease(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0.2.0", newest.Version)
	require.Len(t, feed.seen(), 2)

	pages := make([]string, releaseFeedPages+1)
	for i := range pages {
		pages[i] = full
	}
	long := newRecordingFeedInternal(t, pages...)
	_, _, err = NewVersionService(long.client, false, "", "1.0.0", "", nil, nil).NewestRelease(t.Context())
	require.Equal(t, "update.check-unavailable", codeOf(err))
	require.Len(t, long.seen(), releaseFeedPages)
}

func TestReleaseFeedCachedListIsNotSharedWithCallers(t *testing.T) {
	svc := NewVersionService(newRecordingFeedInternal(t, `[`+feedEntryJSON("v1.1.0")+`,`+feedEntryJSON("v1.0.0")+`]`).client, false, "", "1.0.0", "", nil, nil)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			list, err := svc.feed.releases(t.Context())
			if err == nil && len(list) > 0 {
				list[0].Version = fmt.Sprint("mutated-", i)
				list[0], list[len(list)-1] = list[len(list)-1], list[0]
			}
			_, _, _ = svc.NewestRelease(t.Context())
			_, _, _ = svc.ReleaseAtMost(t.Context(), "1.0.0")
		})
	}
	wg.Wait()
	newest, ok, err := svc.NewestRelease(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "1.1.0", newest.Version)
}

func TestPublishedReleaseAtMostKeepsUpdateAllCodes(t *testing.T) {
	svc := NewVersionService(newRecordingFeedInternal(t, "[]").client, false, "", "1.0.0", "", nil, nil)
	_, err := svc.PublishedReleaseAtMost(t.Context(), "1.0.0")
	require.Equal(t, "update.requires-hub-update", imageDTO.Neutral(err, "", 0).Problem.Code)
}

func TestReleaseFeedChecksEveryRedirectHop(t *testing.T) {
	var requested []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requested = append(requested, r.URL.String())
		if len(requested) == 1 {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"http://192.0.2.10/releases"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("[" + feedEntryJSON("v1.0.0") + "]")), Request: r}, nil
	})}
	svc := NewVersionService(client, false, "https://feed.example.com/releases", "1.0.0", "", nil, nil)
	_, _, err := svc.NewestRelease(t.Context())
	require.Equal(t, "update.check-unavailable", codeOf(err))
	require.Len(t, requested, 1, "a redirect to public cleartext is never requested")
}

func TestReleaseFeedRechecksTheHostBeforeEachPage(t *testing.T) {
	full := "[" + strings.TrimSuffix(strings.Repeat(feedEntryJSON("v1.0.0")+",", releaseFeedPageSize), ",") + "]"
	feed := newRecordingFeedInternal(t, full, "[]")
	svc := NewVersionService(feed.client, false, "http://feed.example.com/releases", "1.0.0", "", nil, nil)
	lookups := 0
	svc.feed.lookup = func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		if lookups == 1 {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("192.0.2.10")}, nil
	}
	_, _, err := svc.NewestRelease(t.Context())
	require.Equal(t, "update.check-unavailable", codeOf(err))
	require.Len(t, feed.seen(), 1, "the second page is refused once the host resolves publicly")
}
