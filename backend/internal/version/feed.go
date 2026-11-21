// Source: docklet-next/issues/235
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Docklet Next release feed client and selectors; sequence-ordered cache with throttled forced refresh for explicit rechecks.

package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
	productVersion "github.com/florianfinn/docklet-next/types/version"
)

// DefaultReleaseFeedURL lists the product's GitHub Releases. DOCKLET_RELEASE_FEED_URL
// replaces it for mirrors and tests.
const DefaultReleaseFeedURL = "https://api.github.com/repos/florianfinn/docklet-next/releases"

const (
	releaseFeedPages     = 10
	releaseFeedPageSize  = 100
	releaseFeedPageBytes = 8 << 20
	releaseFeedTTL       = 3 * time.Hour
	// A list this old is still served while a refetch fails.
	releaseFeedStale = 24 * time.Hour
	// After a failed fetch, reads wait this long before the next attempt.
	releaseFeedRetry = time.Minute
	// Forced refreshes closer together reuse the previous result, which keeps
	// repeated clicks within the unauthenticated GitHub API budget.
	releaseFeedMinRefresh = 30 * time.Second
	// Ordinary reads wait for their fetch; this bounds that wait.
	releaseFeedFetchTimeout = 10 * time.Second
	// The browser gives the refresh 12 seconds; it never waits for another fetch.
	releaseFeedRefreshTimeout = 8 * time.Second
)

// Release is one published stable product release. Values are immutable once
// cached; selectors return copies.
type Release struct {
	Version     string
	Tag         string
	URL         string
	Notes       string
	PublishedAt string
	parsed      productVersion.Product
}

// Selector errors are fresh values so callers may wrap or annotate them.
func errReleaseFeedDisabled() error    { return imageDTO.Failure("feature-disabled", 503) }
func errReleaseFeedUnavailable() error { return imageDTO.Failure("update.check-unavailable", 503) }
func errProductVersionUnknown() error  { return imageDTO.Failure("update.system-version-unknown", 503) }

func failureCodeInternal(err error) string {
	var problem *imageDTO.OperationError
	if errors.As(err, &problem) {
		return problem.Problem.Code
	}
	return ""
}

type releaseFeed struct {
	endpoint *url.URL
	invalid  error
	client   *http.Client
	disabled bool
	// lookup resolves http feed hosts per request; tests replace it.
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)

	// mu guards the fields below and is never held during a fetch. Every fetch
	// takes a sequence number at its start; only a later start may replace the
	// stored list, so an older fetch that finishes last cannot undo a newer one.
	mu        sync.Mutex
	started   uint64
	stored    uint64
	list      []Release
	listAt    time.Time // last stored successful fetch
	failedAt  time.Time // last failed fetch
	forcedAt  time.Time // last explicit refresh
	forcedErr error
	// loadMu lets ordinary reads share one fetch; refreshMu serialises explicit
	// refreshes. Neither waits for the other, so a refresh never queues behind
	// a slow ordinary fetch.
	loadMu    sync.Mutex
	refreshMu sync.Mutex
}

func newReleaseFeed(client *http.Client, disabled bool, raw string) *releaseFeed {
	f := &releaseFeed{client: client, disabled: disabled, lookup: lookupFeedHostInternal}
	if strings.TrimSpace(raw) == "" {
		raw = DefaultReleaseFeedURL
	}
	f.endpoint, f.invalid = parseReleaseFeedURL(raw)
	return f
}

// load fetches the list and stores it unless a later-started fetch already
// stored one. The fetch does not end with the request that triggered it.
func (f *releaseFeed) load(ctx context.Context, timeout time.Duration) error {
	f.mu.Lock()
	f.started++
	seq := f.started
	f.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	releases, err := f.fetch(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.failedAt = time.Now()
		return errReleaseFeedUnavailable()
	}
	if seq > f.stored {
		f.list, f.listAt, f.failedAt, f.stored = releases, time.Now(), time.Time{}, seq
	}
	return nil
}

// due reports whether an ordinary read should fetch: the list is past its TTL
// and the last failure is older than the retry wait.
func (f *releaseFeed) due() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	fresh := !f.listAt.IsZero() && time.Since(f.listAt) < releaseFeedTTL
	waiting := !f.failedAt.IsZero() && time.Since(f.failedAt) < releaseFeedRetry
	return !fresh && !waiting
}

// parseReleaseFeedURL accepts https anywhere and http for hosts that are checked
// per request to be loopback, private (RFC 1918, RFC 4193) or link-local.
func parseReleaseFeedURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("release feed URL must be an absolute http(s) URL without credentials")
	}
	return u, nil
}

func lookupFeedHostInternal(ctx context.Context, host string) ([]netip.Addr, error) {
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	for i := range addrs {
		addrs[i] = addrs[i].Unmap()
	}
	return addrs, err
}

func localFeedAddressInternal(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast()
}

// checkRequestURLInternal applies the scheme rule to every outgoing request:
// each page and each redirect hop, with a fresh lookup for http host names.
func (f *releaseFeed) checkRequestURLInternal(ctx context.Context, u *url.URL) error {
	switch u.Scheme {
	case "https":
		return nil
	case "http":
	default:
		return errors.New("release feed request scheme is not http(s)")
	}
	host := u.Hostname()
	if literal, err := netip.ParseAddr(host); err == nil {
		if localFeedAddressInternal(literal) {
			return nil
		}
		return errors.New("http release feed host is not local")
	}
	addrs, err := f.lookup(ctx, host)
	if err != nil || len(addrs) == 0 {
		return errors.New("http release feed host does not resolve")
	}
	for _, a := range addrs {
		if !localFeedAddressInternal(a) {
			return errors.New("http release feed host resolves to a public address")
		}
	}
	return nil
}

// requestClientInternal shares the transport but checks every redirect hop.
func (f *releaseFeed) requestClientInternal() *http.Client {
	client := *f.client
	previous := f.client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := f.checkRequestURLInternal(req.Context(), req.URL); err != nil {
			return err
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return errors.New("release feed redirected too often")
		}
		return nil
	}
	return &client
}

// releases returns a copy of the cached stable releases, newest first. An empty
// list means no release is published (including a 404 feed).
func (f *releaseFeed) releases(ctx context.Context) ([]Release, error) {
	if f == nil || f.disabled {
		return nil, errReleaseFeedDisabled()
	}
	if f.invalid != nil {
		return nil, errReleaseFeedUnavailable()
	}
	if f.due() {
		f.loadMu.Lock()
		if f.due() {
			_ = f.load(ctx, releaseFeedFetchTimeout)
		}
		f.loadMu.Unlock()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listAt.IsZero() || time.Since(f.listAt) >= releaseFeedStale {
		return nil, errReleaseFeedUnavailable()
	}
	return slices.Clone(f.list), nil
}

// refresh fetches the list again for an explicit recheck. A disabled or invalid
// feed is left to releases, which reports it on the following read.
func (f *releaseFeed) refresh(ctx context.Context) error {
	if f == nil || f.disabled || f.invalid != nil {
		return nil
	}
	f.refreshMu.Lock()
	defer f.refreshMu.Unlock()
	f.mu.Lock()
	recent, last := !f.forcedAt.IsZero() && time.Since(f.forcedAt) < releaseFeedMinRefresh, f.forcedErr
	f.mu.Unlock()
	if recent {
		return last
	}
	err := f.load(ctx, releaseFeedRefreshTimeout)
	f.mu.Lock()
	f.forcedAt, f.forcedErr = time.Now(), err
	f.mu.Unlock()
	return err
}

type feedEntryInternal struct {
	Tag         string `json:"tag_name"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
}

func (f *releaseFeed) fetch(ctx context.Context) ([]Release, error) {
	client := f.requestClientInternal()
	var out []Release
	for page := 1; page <= releaseFeedPages; page++ {
		endpoint := *f.endpoint
		query := endpoint.Query()
		query.Set("per_page", strconv.Itoa(releaseFeedPageSize))
		query.Set("page", strconv.Itoa(page))
		endpoint.RawQuery = query.Encode()
		if err := f.checkRequestURLInternal(ctx, &endpoint); err != nil {
			return nil, err
		}
		entries, missing, err := f.fetchPageInternal(ctx, client, endpoint.String())
		if err != nil {
			return nil, err
		}
		if missing && page == 1 {
			return nil, nil
		}
		if missing {
			return nil, errors.New("release feed page disappeared")
		}
		for _, entry := range entries {
			if release, ok := releaseFromEntryInternal(entry); ok {
				out = append(out, release)
			}
		}
		if len(entries) < releaseFeedPageSize {
			slices.SortStableFunc(out, func(a, b Release) int { return b.parsed.Compare(a.parsed) })
			return out, nil
		}
	}
	// A feed longer than the page cap is incomplete; never guess past it.
	return nil, errors.New("release feed exceeds the page limit")
}

func (f *releaseFeed) fetchPageInternal(ctx context.Context, client *http.Client, endpoint string) ([]feedEntryInternal, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, false, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(request)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return nil, true, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("release feed returned status %d", response.StatusCode)
	}
	var entries []feedEntryInternal
	body := io.LimitReader(response.Body, releaseFeedPageBytes+1)
	data, err := io.ReadAll(body)
	if err != nil || len(data) > releaseFeedPageBytes {
		return nil, false, errors.New("release feed page is unreadable or too large")
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, false, fmt.Errorf("decode release feed: %w", err)
	}
	return entries, false, nil
}

// releaseFromEntryInternal keeps only published stable vX.Y.Z tags. The link
// comes from the entry itself and must be an absolute http(s) URL.
func releaseFromEntryInternal(entry feedEntryInternal) (Release, bool) {
	if entry.Draft || entry.Prerelease || entry.PublishedAt == "" || !strings.HasPrefix(entry.Tag, "v") {
		return Release{}, false
	}
	parsed, ok := productVersion.ParseProduct(entry.Tag)
	if !ok || !parsed.Stable() || "v"+parsed.String() != entry.Tag {
		return Release{}, false
	}
	link := ""
	if u, err := url.Parse(entry.HTMLURL); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
		link = u.String()
	}
	return Release{Version: parsed.String(), Tag: entry.Tag, URL: link, Notes: entry.Body, PublishedAt: entry.PublishedAt, parsed: parsed}, true
}

// RefreshReleases fetches the release list again for an explicit recheck.
func (s *VersionService) RefreshReleases(ctx context.Context) error {
	return s.feed.refresh(ctx)
}

// NewestRelease returns the newest published stable release; ok is false when
// none is published.
func (s *VersionService) NewestRelease(ctx context.Context) (Release, bool, error) {
	releases, err := s.feed.releases(ctx)
	if err != nil || len(releases) == 0 {
		return Release{}, false, err
	}
	return releases[0], true, nil
}

// ReleaseAtMost returns the newest published stable release not newer than
// ceiling (a product version, prereleases allowed).
func (s *VersionService) ReleaseAtMost(ctx context.Context, ceiling string) (Release, bool, error) {
	limit, valid := productVersion.ParseProduct(ceiling)
	if !valid {
		return Release{}, false, errProductVersionUnknown()
	}
	releases, err := s.feed.releases(ctx)
	if err != nil {
		return Release{}, false, err
	}
	for _, release := range releases {
		if release.parsed.Compare(limit) <= 0 {
			return release, true, nil
		}
	}
	return Release{}, false, nil
}
