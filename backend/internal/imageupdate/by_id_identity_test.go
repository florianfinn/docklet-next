// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Deterministic repository choice for the by-ID convenience read.

package imageupdate

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIdentityByIDTagOrder(t *testing.T) {
	tags := []string{"registry.example.com/z:1", "registry.example.com/a:1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "RepoTags": tags, "RepoDigests": []string{}})
	}))
	defer server.Close()
	svc := &ImageUpdateService{dockerService: &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}}
	for range 2 {
		got, e := svc.ReadUpdateByID(t.Context(), "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		require.NoError(t, e)
		require.Equal(t, "registry.example.com/a:1", got.ImageRef)
		tags[0], tags[1] = tags[1], tags[0]
	}
}

func TestIdentityRepositoryProvenance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "RepoDigests": []string{"registry.example.com/other@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}})
	}))
	defer server.Close()
	svc := &ImageUpdateService{dockerService: &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}}
	got, e := svc.ReadUpdateInfo(t.Context(), "registry.example.com/app:stable")
	require.NoError(t, e)
	require.Equal(t, "local", got.State)
	require.False(t, got.HasUpdate)
}
