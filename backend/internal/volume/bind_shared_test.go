// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type bindSignalContext struct {
	context.Context
	ready chan struct{}
	once  sync.Once
}

func (c *bindSignalContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.ready) })
	return c.Context.Done()
}
func TestBindSharedContainerRowsStayUnchanged(t *testing.T) {
	source := []container.Summary{{ID: "app", Names: []string{"/app"}, Labels: map[string]string{"example": "kept"}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: "/srv/z", Destination: "/z"}, {Type: mount.TypeBind, Source: "/srv/a", Destination: "/a"}}}}
	read, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			once.Do(func() { close(read) })
			<-release
			require.NoError(t, json.NewEncoder(w).Encode(source))
		case strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
		case strings.HasSuffix(r.URL.Path, "/archive"):
			stat, _ := json.Marshal(map[string]any{"name": "data", "size": 0, "mode": uint32(1<<31 | 0755)})
			w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(stat))
			w.WriteHeader(200)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cli := newVolumeServiceTestDockerClientInternal(t, server)
	ds := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)
	svc := &VolumeService{dockerService: ds}
	first := &bindSignalContext{Context: t.Context(), ready: make(chan struct{})}
	second := &bindSignalContext{Context: t.Context(), ready: make(chan struct{})}
	held := make(chan []container.Summary, 2)
	transform := make(chan struct{})
	done := make(chan error, 2)
	run := func(ctx context.Context) {
		rows, err := ds.ListContainers(ctx)
		if err != nil {
			done <- err
			return
		}
		held <- rows
		<-transform
		_, err = svc.bindsFromContainers(ctx, cli, rows)
		done <- err
	}
	go run(first)
	<-read
	go run(second)
	<-first.ready
	<-second.ready
	close(release)
	a, b := <-held, <-held
	require.True(t, &a[0] == &b[0], "both callers consume the real coalesced slice")
	before, err := json.Marshal(a)
	require.NoError(t, err)
	close(transform)
	require.NoError(t, <-done)
	require.NoError(t, <-done)
	after, err := json.Marshal(a)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
	require.Equal(t, source, a)
}
func TestBindScanWaitersAndMemoryResult(t *testing.T) {
	state := (&VolumeService{}).bindState()
	entered := make(chan context.Context, 1)
	finish := make(chan struct{})
	workerDone := make(chan struct{})
	scan := func(ctx context.Context, key string) (volumetypes.BindMeasurement, error) {
		entered <- ctx
		defer close(workerDone)
		select {
		case <-ctx.Done():
			return volumetypes.BindMeasurement{}, ctx.Err()
		case <-finish:
			return volumetypes.BindMeasurement{Key: key, MeasuredSize: 24}, nil
		}
	}
	c1, cancel1 := context.WithCancel(t.Context())
	defer cancel1()
	c2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()
	a := &bindSignalContext{Context: c1, ready: make(chan struct{})}
	b := &bindSignalContext{Context: c2, ready: make(chan struct{})}
	done := make(chan error, 2)
	go func() { _, err := state.measure(a, "key", time.Minute, scan); done <- err }()
	work := <-entered
	go func() { _, err := state.measure(b, "key", time.Minute, scan); done <- err }()
	<-a.ready
	<-b.ready
	cancel1()
	require.ErrorIs(t, <-done, context.Canceled)
	require.NoError(t, work.Err(), "remaining waiter owns the shared run")
	close(finish)
	require.NoError(t, <-done)
	<-workerDone
	state.mu.Lock()
	require.EqualValues(t, 24, state.measured["key"].MeasuredSize)
	state.mu.Unlock()
}
func TestBindScanLastWaiterCancels(t *testing.T) {
	state := (&VolumeService{}).bindState()
	entered := make(chan context.Context, 1)
	removed := make(chan struct{})
	scan := func(ctx context.Context, _ string) (volumetypes.BindMeasurement, error) {
		entered <- ctx
		<-ctx.Done()
		close(removed)
		return volumetypes.BindMeasurement{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := state.measure(ctx, "key", time.Minute, scan); done <- err }()
	work := <-entered
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.ErrorIs(t, work.Err(), context.Canceled)
	<-removed
}
