// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Published stable release selection below the running hub for issue #229.

package version

import (
	"context"
	"encoding/json"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// PublishedReleaseAtMost selects from observed published stable releases. It
// never manufactures a tag from the hub version or guesses past a failed page.
func (s *VersionService) PublishedReleaseAtMost(ctx context.Context, hub string) (string, error) {
	if _, ok := imageDTO.CompareRelease(hub, hub); !ok {
		return "", imageDTO.Failure("update.system-version-unknown", 503)
	}
	if s == nil || s.httpClient == nil {
		return "", imageDTO.Failure("update.requires-hub-update", 409)
	}
	best := ""
	for page := 1; page <= 10; page++ {
		endpoint := strings.TrimSuffix(versionCheckURL, "/latest") + "?per_page=100&page=" + strconv.Itoa(page)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
		if err != nil {
			return "", imageDTO.Failure("update.check-unavailable", 503)
		}
		response, err := s.httpClient.Do(request)
		if err != nil {
			return "", imageDTO.Failure("update.check-unavailable", 503)
		}
		var releases []struct {
			Tag         string `json:"tag_name"`
			Draft       bool   `json:"draft"`
			Prerelease  bool   `json:"prerelease"`
			PublishedAt string `json:"published_at"`
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&releases)
		_ = response.Body.Close()
		if response.StatusCode != 200 || err != nil {
			return "", imageDTO.Failure("update.check-unavailable", 503)
		}
		for _, release := range releases {
			if release.Draft || release.Prerelease || release.PublishedAt == "" {
				continue
			}
			cmp, ok := imageDTO.CompareRelease(release.Tag, hub)
			if !ok || cmp > 0 {
				continue
			}
			if best == "" {
				best = release.Tag
			} else if cmp, _ := imageDTO.CompareRelease(release.Tag, best); cmp > 0 {
				best = release.Tag
			}
		}
		if len(releases) < 100 {
			if best != "" {
				return best, nil
			}
			return "", imageDTO.Failure("update.requires-hub-update", 409)
		}
	}
	return "", imageDTO.Failure("update.check-unavailable", 503)
}
