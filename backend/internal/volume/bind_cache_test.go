// Source: docklet-next/issues/388
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original bind kind cache eviction, failed-stat retry and late-insertion proofs.

package volume

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

func cacheContainer(id, destination string) container.Summary {
	return container.Summary{ID: id, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: "/srv/" + id, Destination: destination}}}
}
func cacheStat(w http.ResponseWriter) {
	stat, _ := json.Marshal(map[string]any{"name": "data", "mode": uint32(1<<31 | 0755)})
	w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(stat))
	w.WriteHeader(http.StatusOK)
}
func TestBindKindCachePrunesOnlySuccessfulFullListings(t *testing.T) {
	rows := []container.Summary{cacheContainer("a", "/data"), cacheContainer("b", "/data")}
	failList := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			if failList {
				w.WriteHeader(503)
				_, _ = w.Write([]byte(`{"message":"unavailable"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(rows)
		case strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
		case strings.HasSuffix(r.URL.Path, "/archive"):
			cacheStat(w)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cli := newVolumeServiceTestDockerClientInternal(t, server)
	svc := &VolumeService{dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)}
	_, err := svc.ListBinds(t.Context())
	require.NoError(t, err)
	state := svc.bindState()
	snapshot := func() map[string]string {
		state.mu.Lock()
		defer state.mu.Unlock()
		result := map[string]string{}
		for k, v := range state.kinds {
			result[k] = v
		}
		return result
	}
	require.Equal(t, map[string]string{"a\x00/data": "directory", "b\x00/data": "directory"}, snapshot())
	failList = true
	rows = rows[1:]
	_, err = svc.ListBinds(t.Context())
	require.Error(t, err)
	require.Len(t, snapshot(), 2, "a failed full listing must preserve known kinds")
	failList = false
	_, err = svc.ListBinds(t.Context())
	require.NoError(t, err)
	require.Equal(t, map[string]string{"b\x00/data": "directory"}, snapshot(), "removed container kinds must be pruned")
	rows = []container.Summary{cacheContainer("b", "/new")}
	_, err = svc.ListBinds(t.Context())
	require.NoError(t, err)
	require.Equal(t, map[string]string{"b\x00/new": "directory"}, snapshot(), "membership includes the mount destination")
}
func TestBindKindCacheRetriesFailedStat(t *testing.T) {
	var stats atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
		case strings.HasSuffix(r.URL.Path, "/archive"):
			if stats.Add(1) == 1 {
				w.WriteHeader(503)
				_, _ = w.Write([]byte(`{"message":"unavailable"}`))
				return
			}
			cacheStat(w)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cli := newVolumeServiceTestDockerClientInternal(t, server)
	svc := &VolumeService{}
	rows := []container.Summary{cacheContainer("a", "/data")}
	result, err := svc.bindsFromContainers(t.Context(), cli, rows)
	require.NoError(t, err)
	require.Equal(t, "unknown", result[0].Kind)
	state := svc.bindState()
	state.mu.Lock()
	cached := len(state.kinds)
	state.mu.Unlock()
	require.Zero(t, cached, "failed stats must not be cached")
	result, err = svc.bindsFromContainers(t.Context(), cli, rows)
	require.NoError(t, err)
	require.Equal(t, "directory", result[0].Kind, "a later successful stat must repair the response")
	require.EqualValues(t, 2, stats.Load())
}
func TestBindKindCacheLateStatCannotResurrectRemovedKey(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
		case strings.HasSuffix(r.URL.Path, "/archive"):
			close(entered)
			<-release
			cacheStat(w)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cli := newVolumeServiceTestDockerClientInternal(t, server)
	svc := &VolumeService{}
	done := make(chan error, 1)
	go func() {
		_, err := svc.bindsFromContainers(t.Context(), cli, []container.Summary{cacheContainer("a", "/data")})
		done <- err
	}()
	<-entered
	// Completing this listing also proves the held Docker stat owns no state lock.
	newer := make(chan error, 1)
	go func() { _, err := svc.bindsFromContainers(t.Context(), cli, nil); newer <- err }()
	select {
	case err := <-newer:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		close(release)
		<-done
		t.Fatal("new listing blocked behind Docker stat")
	}
	close(release)
	require.NoError(t, <-done)
	state := svc.bindState()
	state.mu.Lock()
	defer state.mu.Unlock()
	require.Empty(t, state.kinds, "a stat finishing after removal must not resurrect the key")
	require.Empty(t, state.current)
}
