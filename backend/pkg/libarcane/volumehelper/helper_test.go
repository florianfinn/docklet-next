// Source: getarcaneapp/arcane/backend/pkg/libarcane/volumehelper/helper_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; No-fallback tools image resolution and full references.

package volumehelper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/distribution/reference"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
)

func TestLabels(t *testing.T) {
	labels := Labels()

	require.Equal(t, "true", labels[libarcane.InternalResourceLabel])
	require.Equal(t, "true", labels[ContainerLabel])
	require.Len(t, labels, 2)
}

func TestResolveHelperImage_UsesLocalToolsImage(t *testing.T) {
	var pullCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Id": "tools-image"})) {
				return
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create"):
			pullCalls.Add(1)
			http.Error(w, "unexpected pull", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	image, err := ResolveHelperImage(t.Context(), newTestDockerClientInternal(t, server), ToolsImage(""))

	require.NoError(t, err)
	require.Equal(t, ToolsImage(""), image)
	require.Zero(t, pullCalls.Load())
}

func TestResolveHelperImage_PullsToolsImageWhenMissing(t *testing.T) {
	var pullCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create"):
			pullCalls.Add(1)
			if !assert.Equal(t, "ghcr.io/florianfinn/docklet-tools", r.URL.Query().Get("fromImage")) {
				return
			}
			if !assert.Equal(t, "sha256:d387beb35707c5634dba1630b4153c5bfe0bd4529025297ce7c024c2f2fe9602", r.URL.Query().Get("tag")) {
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte("{\"status\":\"pulled\"}\n"))
			if !assert.NoError(t, err) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	image, err := ResolveHelperImage(t.Context(), newTestDockerClientInternal(t, server), ToolsImage(""))

	require.NoError(t, err)
	require.Equal(t, ToolsImage(""), image)
	require.EqualValues(t, 1, pullCalls.Load())
}

func TestResolveHelperImage_ReturnsPullErrorWithoutRuntimeFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create"):
			http.Error(w, "pull failed", http.StatusInternalServerError)
		case strings.Contains(r.URL.Path, "/containers/"):
			t.Error("helper resolution must never inspect runtime containers")
			w.WriteHeader(500)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	image, err := ResolveHelperImage(t.Context(), newTestDockerClientInternal(t, server), ToolsImage(""))

	require.Error(t, err)
	require.Empty(t, image)
	require.ErrorContains(t, err, "failed to resolve helper image")
	require.ErrorContains(t, err, "pull failed")
}

func newTestDockerClientInternal(t *testing.T, server *httptest.Server) *client.Client {
	t.Helper()

	dockerClient, err := client.New(
		client.WithHost(server.URL),
		client.WithAPIVersion("1.54"),
	)
	require.NoError(t, err)
	return dockerClient
}

func TestToolsImageUsesCompleteOverride(t *testing.T) {
	require.Equal(t, DefaultToolsImage, ToolsImage("  "))
	require.Equal(t, "registry.example.com/tools:latest", ToolsImage(" registry.example.com/tools:latest "))
	config := HostConfig("registry.example.com/tools:latest", []string{"data:/volume"}, nil)
	require.Empty(t, config.Tmpfs)
	require.Equal(t, []string{"data:/volume"}, config.Binds)
}

func TestResolveHelperImageRejectsPullStreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/images/create") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errorDetail":{"message":"synthetic pull failure"}}`))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(server.Close)
	image, err := ResolveHelperImage(t.Context(), newTestDockerClientInternal(t, server), "registry.example.com/tools:latest")
	require.ErrorContains(t, err, "helper image pull failed")
	require.Empty(t, image)
}

func TestDefaultToolsImageIsPinnedReference(t *testing.T) {
	named, err := reference.ParseNormalizedNamed(DefaultToolsImage)
	require.NoError(t, err)
	tagged, ok := named.(reference.NamedTagged)
	require.True(t, ok, "release default must include a SemVer tag")
	require.Equal(t, "0.1.0", tagged.Tag())
	digested, ok := named.(reference.Digested)
	require.True(t, ok, "release default must include the multi-platform index digest")
	require.Equal(t, "sha256:d387beb35707c5634dba1630b4153c5bfe0bd4529025297ce7c024c2f2fe9602", digested.Digest().String())
}
