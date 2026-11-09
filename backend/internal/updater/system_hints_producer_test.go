// Source: docklet-next/issues/235
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original producer-coupled hint test (M21 R1 S23-R1 reproduction).

package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	versionService "github.com/florianfinn/docklet-next/backend/internal/version"
)

// The real app-version producer reports RepoDigests as a repository-only
// reference and the tag separately; hints must consume exactly that shape.
func TestSystemImageHintsConsumeTheRealAppVersionImageMetadata(t *testing.T) {
	repo := "ghcr.io/florianfinn/docklet-next-agent"
	digest := "sha256:" + strings.Repeat("a", 64)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/v1.49")
		switch {
		case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": strings.Repeat("b", 64), "Name": "/docklet-agent", "Image": digest, "Config": map[string]any{"Image": repo + ":1.0.0@" + digest}})
		case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": digest, "RepoDigests": []string{repo + "@" + digest}})
		case path == "/containers/json":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"Id": strings.Repeat("b", 64), "Names": []string{"/docklet-agent"}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engine.Close()
	c, err := client.New(client.WithHost(engine.URL), client.WithAPIVersion("1.49"))
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	dockerService := docker.NewDockerClientService(t.Context(), nil, &config.Config{}, nil).WithClient(c)
	info := versionService.NewVersionService(nil, true, "", "1.0.0", "", dockerService, nil).GetAppVersionInfo(t.Context())
	require.Equal(t, repo, info.CurrentImageRef)
	require.Equal(t, "1.0.0", info.CurrentTag)

	for _, tc := range []struct {
		name, state, code string
		feed              releaseFeedObservation
	}{
		{"available", "available", "", releaseFeedObservation{latest: "1.1.0", best: "1.1.0"}},
		{"current", "current", "", releaseFeedObservation{latest: "1.0.0", best: "1.0.0"}},
		{"disabled", "unknown", "feature-disabled", releaseFeedObservation{code: "feature-disabled"}},
		{"feed error", "unknown", "update.check-unavailable", releaseFeedObservation{code: "update.check-unavailable"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := systemImageHints("1.1.0", repo+":1.1.0@"+digest, repo, *info, tc.feed)
			require.Equal(t, tc.state, out.Images[0].State)
			require.Equal(t, tc.code, out.Images[0].Code)
		})
	}
	// A tag that disagrees with the running version stays unknown.
	mismatch := *info
	mismatch.CurrentTag = "0.9.0"
	require.Equal(t, "unknown", systemImageHints("1.1.0", repo+":1.1.0", repo, mismatch, releaseFeedObservation{best: "1.1.0"}).Images[0].State)
}
