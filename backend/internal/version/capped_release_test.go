// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Published release feed selection and failure regression coverage.

package version

import (
	"encoding/json"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

type cappedTransport func(*http.Request) (*http.Response, error)

func (f cappedTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPublishedReleaseSelectionUsesAllPagesAndStableEvidence(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: cappedTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var releases []map[string]any
		if r.URL.Query().Get("page") == "1" {
			for range 100 {
				releases = append(releases, map[string]any{"tag_name": "1.9.0", "published_at": "2026-10-24T00:00:00Z"})
			}
		} else {
			releases = []map[string]any{
				{"tag_name": "2.0.0", "published_at": "2026-10-24T00:00:00Z"},
				{"tag_name": "2.1.0", "published_at": "2026-10-24T00:00:00Z"},
				{"tag_name": "2.0.1", "published_at": "2026-10-24T00:00:00Z", "draft": true},
				{"tag_name": "2.0.2", "published_at": "2026-10-24T00:00:00Z", "prerelease": true},
				{"tag_name": "2.0.3"},
			}
		}
		body, err := json.Marshal(releases)
		require.NoError(t, err)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})}
	service := NewVersionService(client, true, "2.0.9", "", nil, nil, nil, nil)
	selected, err := service.PublishedReleaseAtMost(t.Context(), "2.0.9")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", selected)
	require.Equal(t, 2, calls)
}
func TestPublishedReleaseSelectionRefusesPartialFeed(t *testing.T) {
	calls := 0
	service := NewVersionService(&http.Client{Transport: cappedTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		status := 200
		body := "[" + strings.TrimSuffix(strings.Repeat(`{"tag_name":"1.9.0","published_at":"2026-10-24T00:00:00Z"},`, 100), ",") + "]"
		if calls == 2 {
			status = 503
			body = `{}`
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}, true, "2.0.0", "", nil, nil, nil, nil)
	selected, err := service.PublishedReleaseAtMost(t.Context(), "2.0.0")
	require.Empty(t, selected)
	require.Equal(t, "update.check-unavailable", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Equal(t, 2, calls)
}
