// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original tests for the explicit upgrade target and Docklet role detection.

package upgrade

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestValidateUpgradeTargetInternal(t *testing.T) {
	cases := []struct {
		name, image, pull, role string
		ok                      bool
	}{
		{"missing image", "", "", "hub", false},
		{"blank image", "  ", "", "hub", false},
		{"release pin", "ghcr.io/florianfinn/docklet-next-hub:1.0.0@sha256:" + strings.Repeat("a", 64), "", "hub", true},
		{"operator latest is allowed", "ghcr.io/florianfinn/docklet-next-agent:latest", "", "agent", true},
		{"own registry with port", "registry.example.com:5000/docklet/hub:1.0.0", "", "hub", true},
		{"invalid reference", "UPPER/Case:tag", "", "hub", false},
		{"invalid pull reference", "ghcr.io/florianfinn/docklet-next-hub:1.0.0", "bad ref!", "hub", false},
		{"digest-only reference", "registry.example.com/docklet/hub@sha256:" + strings.Repeat("a", 64), "", "hub", true},
		{"malformed digest", "registry.example.com/docklet/hub:1.0.0@sha256:not-a-digest", "", "hub", false},
		{"short digest", "registry.example.com/docklet/hub:1.0.0@sha256:abc", "", "hub", false},
		{"empty digest", "registry.example.com/docklet/hub:1.0.0@", "", "hub", false},
		{"unsupported digest", "registry.example.com/docklet/hub:1.0.0@md5:" + strings.Repeat("a", 32), "", "hub", false},
		{"malformed pull digest", "ghcr.io/florianfinn/docklet-next-hub:1.0.0", "registry.example.com/docklet/hub:1.0.0@sha256:not-a-digest", "hub", false},
		{"unknown role", "ghcr.io/florianfinn/docklet-next-hub:1.0.0", "", "proxy", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUpgradeTargetInternal(tc.image, tc.pull, tc.role)
			require.Equal(t, tc.ok, err == nil, "error: %v", err)
		})
	}
}

func TestRunUpgradeInvalidTargetMakesNoEngineRequest(t *testing.T) {
	var requests atomic.Int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer engine.Close()
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(engine.URL, "http://"))

	saved := []string{targetImage, pullImageRef, targetRole, containerName}
	defer func() { targetImage, pullImageRef, targetRole, containerName = saved[0], saved[1], saved[2], saved[3] }()
	for _, tc := range []struct{ image, pull, want string }{
		{"", "", "--image is required"},
		{"registry.example.com/docklet/hub:1.0.0@sha256:not-a-digest", "", "invalid --image"},
		{"registry.example.com/docklet/hub:1.0.0", "registry.example.com/docklet/hub:1.0.0@sha256:abc", "invalid --pull-image"},
	} {
		targetImage, pullImageRef, targetRole, containerName = tc.image, tc.pull, "hub", ""
		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		require.ErrorContains(t, runUpgrade(cmd, nil), tc.want)
		require.Zero(t, requests.Load(), tc.image)
	}
}

type engineContainerInternal struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
}

func newRoleEngineInternal(t *testing.T, items []engineContainerInternal, filters *atomic.Value) *client.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/v1.49")
		switch {
		case path == "/containers/json":
			filters.Store(r.URL.Query().Get("filters"))
			_ = json.NewEncoder(w).Encode(items)
		case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/" + id, "Config": map[string]any{"Labels": map[string]string{dockletSystemLabel: "hub"}}})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"unexpected"}`)
		}
	}))
	t.Cleanup(server.Close)
	c, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.49"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestFindDockletContainerUsesRoleLabelAndSkipsHelpers(t *testing.T) {
	var filters atomic.Value
	c := newRoleEngineInternal(t, []engineContainerInternal{
		{ID: "helper", Names: []string{"/docklet-self-update-1"}, Labels: map[string]string{dockletSystemLabel: "hub", dockletHelperLabel: "self-update"}},
		{ID: "hub", Names: []string{"/docklet-hub"}, Labels: map[string]string{dockletSystemLabel: "hub"}},
	}, &filters)

	found, err := findDockletContainer(t.Context(), c, "hub")
	require.NoError(t, err)
	require.Equal(t, "hub", found.ID)
	require.Contains(t, filters.Load().(string), `"io.docklet.system=hub"`)
}

func TestFindDockletContainerRefusesAmbiguousOrMissingRole(t *testing.T) {
	var filters atomic.Value
	two := newRoleEngineInternal(t, []engineContainerInternal{
		{ID: "a", Names: []string{"/docklet-agent-a"}, Labels: map[string]string{dockletSystemLabel: "agent"}},
		{ID: "b", Names: []string{"/docklet-agent-b"}, Labels: map[string]string{dockletSystemLabel: "agent"}},
	}, &filters)
	_, err := findDockletContainer(t.Context(), two, "agent")
	require.ErrorContains(t, err, "choose one with --container")

	none := newRoleEngineInternal(t, nil, &filters)
	_, err = findDockletContainer(t.Context(), none, "hub")
	require.ErrorContains(t, err, "no running container labelled io.docklet.system=hub")
}
