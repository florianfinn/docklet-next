// Source: docklet-next/issues/386
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original streamed download proofs through the real agent stack and hub Direct forwarding.

package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

type streamRig struct {
	t         *testing.T
	f         *runtimePlan
	archive   []byte
	mkdirHold chan struct{}
	// mkdirEntered and tarStart signal that the helper exists and the restore or tar phase began.
	mkdirEntered chan struct{}
	tarHold      chan struct{}
	tarFail      error
	tarStart     chan struct{}
	mu           sync.Mutex
	tarCmd       []string
	client       *http.Client
	url          string
	hub          *httptest.Server
}

// newStreamRig serves the registered backup routes on an agent stack (Echo Recover, Huma) behind the hub's
// production Direct forwarding, both over the given protocol.
func newStreamRig(t *testing.T, proto string) *streamRig {
	t.Helper()
	f := newRuntimePlan(t)
	db := f.service.deps.DB
	require.NoError(t, db.AutoMigrate(activity.Models()...))
	require.NoError(t, db.AutoMigrate(&backuptarget.Target{}, &policies.VolumeBackupPolicy{}))
	f.service.deps.Activity = activity.NewActivityService(db, nil)
	f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) {
		return &volume.Backup{ID: "row", VolumeName: "data", Format: volume.BackupFormatRustic, Destination: "local", LocalSnapshotID: strings.Repeat("a", 64), Size: 1}, nil
	}
	f.service.deps.DownloadKeepAlive = 25 * time.Millisecond
	r := &streamRig{t: t, f: f, archive: sampleArchive(t), tarStart: make(chan struct{}, 1), mkdirEntered: make(chan struct{}, 1)}
	exec := f.service.deps.Exec
	f.service.deps.Exec = func(ctx context.Context, id, execUser string, command []string) (string, string, error) {
		if command[0] == "mkdir" {
			select {
			case r.mkdirEntered <- struct{}{}:
			default:
			}
		}
		if command[0] == "mkdir" && r.mkdirHold != nil {
			select {
			case <-r.mkdirHold:
			case <-ctx.Done():
				return "", "", ctx.Err()
			}
		}
		return exec(ctx, id, execUser, command)
	}
	f.service.deps.ExecStream = func(ctx context.Context, _ string, command []string, stdout io.Writer) error {
		r.mu.Lock()
		r.tarCmd = command
		r.mu.Unlock()
		half := len(r.archive) / 2
		if _, err := stdout.Write(r.archive[:half]); err != nil {
			return err
		}
		select {
		case r.tarStart <- struct{}{}:
		default:
		}
		if r.tarHold != nil {
			select {
			case <-r.tarHold:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if r.tarFail != nil {
			return r.tarFail
		}
		_, err := stdout.Write(r.archive[half:])
		return err
	}
	agent := echo.New()
	agent.Use(echomiddleware.Recover())
	api := humaecho.NewWithGroup(agent, agent.Group("/api"), huma.DefaultConfig("backup", "test"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
		next(huma.WithContext(ctx, request))
	})
	RegisterRoutes(api, NewHandler(f.service, f.service.deps.Activity, nil, nil, t.Context(), &config.Config{AgentMode: true}))
	r.hub = proxyHub(t, streamServer(t, agent, proto), proto)
	r.client, r.url = r.hub.Client(), r.hub.URL+"/api/environments/example-storage/volumes/backups/row/download"
	return r
}

// proxyHub serves the hub's production Direct forwarding in front of agentServer.
func proxyHub(t *testing.T, agentServer *httptest.Server, proto string) *httptest.Server {
	t.Helper()
	hub := echo.New()
	hub.Use(echomiddleware.Recover())
	hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id",
		func(context.Context, string) (string, *string, bool, error) {
			token := "example"
			return agentServer.URL, &token, true, nil
		},
		nil, nil, edge.NewTunnelRegistry(), nil, middleware.EnvironmentProxySecurity{HTTPClient: agentServer.Client()}))
	hub.GET("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(500) })
	return streamServer(t, hub, proto)
}

func streamServer(t *testing.T, handler http.Handler, proto string) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	if proto == "h2" {
		server.EnableHTTP2 = true
		server.StartTLS()
	} else {
		server.Start()
	}
	t.Cleanup(server.Close)
	return server
}

func sampleArchive(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}))
	content := bytes.Repeat([]byte("snapshot data "), 4096)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "./data.txt", Mode: 0o644, Size: int64(len(content))}))
	_, err := writer.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

// streamBody reads the response on its own goroutine so the test can observe bytes while the producer is held.
type streamBody struct {
	mu    sync.Mutex
	data  []byte
	done  chan struct{}
	err   error
	bytes atomic.Int64
}

func readStream(response *http.Response) *streamBody {
	body := &streamBody{done: make(chan struct{})}
	go func() {
		defer close(body.done)
		buffer := make([]byte, 32<<10)
		for {
			n, err := response.Body.Read(buffer)
			if n > 0 {
				body.mu.Lock()
				body.data = append(body.data, buffer[:n]...)
				body.mu.Unlock()
				body.bytes.Add(int64(n))
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					body.err = err
				}
				return
			}
		}
	}()
	return body
}

func (b *streamBody) waitBytes(t *testing.T, above int64) int64 {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && b.bytes.Load() <= above {
		time.Sleep(5 * time.Millisecond)
	}
	return b.bytes.Load()
}

func (b *streamBody) finish(t *testing.T) ([]byte, error) {
	t.Helper()
	select {
	case <-b.done:
	case <-time.After(5 * time.Second):
		t.Fatal("download body did not end")
	}
	return b.data, b.err
}

func (r *streamRig) get(ctx context.Context) *http.Response {
	r.t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	require.NoError(r.t, err)
	response, err := r.client.Do(request)
	require.NoError(r.t, err)
	r.t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func (r *streamRig) activityStatus() activitytypes.Status {
	r.t.Helper()
	var row activity.Activity
	require.NoError(r.t, r.f.service.deps.DB.Where("resource_type = ?", "volume_backup_download").First(&row).Error)
	return row.Status
}

func (r *streamRig) activityError() string {
	r.t.Helper()
	var row activity.Activity
	require.NoError(r.t, r.f.service.deps.DB.Where("resource_type = ?", "volume_backup_download").First(&row).Error)
	require.NotNil(r.t, row.Error)
	return *row.Error
}

func (r *streamRig) requireReleased() {
	r.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		lease, ok, err := r.f.service.deps.Engine.TryAcquireRun(r.t.Context(), backup.VolumeAdmissionScope, "data")
		require.NoError(r.t, err)
		if ok {
			lease.Release(r.t.Context())
			unlock, locked := r.f.service.deps.Locks.TryLock("data")
			require.True(r.t, locked)
			unlock()
			return
		}
		require.True(r.t, time.Now().Before(deadline), "download admission was not released")
		time.Sleep(10 * time.Millisecond)
	}
}

func gunzip(t *testing.T, data []byte) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	plain, err := io.ReadAll(reader)
	require.NoError(t, err)
	return plain
}

// Headers and the gzip header arrive while the restore is held, sync blocks keep arriving on every pulse, and
// the completed body is exactly the helper's tar stream in one gzip member.
func TestStreamedDownloadCommitsEarlyAndKeepsAlive(t *testing.T) {
	for _, proto := range []string{"h1", "h2"} {
		t.Run(proto, func(t *testing.T) {
			r := newStreamRig(t, proto)
			r.mkdirHold = make(chan struct{})
			released := false
			defer func() {
				if !released {
					close(r.mkdirHold)
				}
			}()
			responses := make(chan *http.Response, 1)
			go func() { responses <- r.get(t.Context()) }()
			var response *http.Response
			select {
			case response = <-responses:
			case <-time.After(2 * time.Second):
				require.Fail(t, "response headers did not arrive while the restore was held")
			}
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.EqualValues(t, -1, response.ContentLength)
			require.Equal(t, "application/x-gzip", response.Header.Get("Content-Type"))
			body := readStream(response)
			first := body.waitBytes(t, 0)
			require.GreaterOrEqual(t, first, int64(10), "gzip header before the restore finishes")
			pulsed := body.waitBytes(t, first+10)
			require.Greater(t, pulsed, first+10, "keep-alive bytes while the restore is held")
			released = true
			close(r.mkdirHold)
			data, err := body.finish(t)
			require.NoError(t, err)
			require.Equal(t, r.archive, gunzip(t, data))
			r.mu.Lock()
			require.Equal(t, []string{"tar", "-cf", "-", "-C", "/scratch/data", "."}, r.tarCmd)
			r.mu.Unlock()
			r.requireReleased()
			require.Equal(t, activitytypes.StatusSuccess, r.activityStatus())
			_, _, removed := r.f.effects()
			require.Contains(t, removed, "helper")
			require.Contains(t, removed, "scratch")
		})
	}
}

// A failure after the 200 aborts the body on both hops; the client never sees a complete response.
func TestStreamedDownloadFailureAfterCommitAborts(t *testing.T) {
	for _, proto := range []string{"h1", "h2"} {
		for _, failure := range []string{"restore", "tar"} {
			t.Run(proto+"/"+failure, func(t *testing.T) {
				r := newStreamRig(t, proto)
				if failure == "restore" {
					r.f.exit = 1
				} else {
					r.tarFail = errors.New("tar exited with status 2")
				}
				response := r.get(t.Context())
				require.Equal(t, http.StatusOK, response.StatusCode)
				_, err := readStream(response).finish(t)
				require.Error(t, err, "a failed download must not end like a complete body")
				r.requireReleased()
				require.Equal(t, activitytypes.StatusFailed, r.activityStatus())
				require.Equal(t, "volume.backup.repository-unavailable", r.activityError())
				_, _, removed := r.f.effects()
				require.Contains(t, removed, "scratch")
			})
		}
	}
}

// Refusals before the stream stay typed problems and leave no helper, activity or admission behind.
func TestStreamedDownloadAdmissionRefusalsStayTyped(t *testing.T) {
	for _, refusal := range []string{"busy", "missing-snapshot"} {
		t.Run(refusal, func(t *testing.T) {
			r := newStreamRig(t, "h1")
			if refusal == "busy" {
				lease, ok, err := r.f.service.deps.Engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, "data")
				require.NoError(t, err)
				require.True(t, ok)
				defer lease.Release(t.Context())
			} else {
				r.f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) {
					return &volume.Backup{ID: "row", VolumeName: "data", Format: volume.BackupFormatRustic}, nil
				}
			}
			response := r.get(t.Context())
			payload, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			if refusal == "busy" {
				require.Equal(t, http.StatusConflict, response.StatusCode, string(payload))
				require.Contains(t, string(payload), "volume.backup.busy")
			} else {
				require.Equal(t, http.StatusNotFound, response.StatusCode, string(payload))
				require.Contains(t, string(payload), "volume.backup.snapshot-not-found")
			}
			creates, _, _ := r.f.effects()
			require.Empty(t, creates)
			var running int64
			require.NoError(t, r.f.service.deps.DB.Model(&activity.Activity{}).Where("status = ?", activitytypes.StatusRunning).Count(&running).Error)
			require.Zero(t, running)
		})
	}
}

// Client cancellation and owner cancellation (maintenance) end the stream in the restore and the tar phase;
// the admission outlives an unconfirmed helper removal.
func TestStreamedDownloadCancellationConfirmsCleanupBeforeRelease(t *testing.T) {
	for _, cause := range []string{"client", "owner"} {
		for _, phase := range []string{"restore", "tar"} {
			t.Run(cause+"/"+phase, func(t *testing.T) {
				r := newStreamRig(t, "h1")
				if phase == "restore" {
					r.mkdirHold = make(chan struct{})
				} else {
					r.tarHold = make(chan struct{})
				}
				held := &atomic.Bool{}
				// The restore engine removes and confirms its own containers; in the tar case only the helper is left.
				held.Store(phase == "restore")
				r.f.cleanupHeld = held
				r.f.cleanupInspect = make(chan struct{}, 1)
				defer held.Store(false)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				response := r.get(ctx)
				body := readStream(response)
				entered := r.mkdirEntered
				if phase == "tar" {
					entered = r.tarStart
				}
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal(phase + " phase did not start")
				}
				held.Store(true)
				drained := make(chan error, 1)
				if cause == "client" {
					cancel()
				} else {
					go func() { drained <- r.f.service.deps.Activity.CancelAndDrain(t.Context(), "") }()
				}
				select {
				case <-r.f.cleanupInspect:
				case <-time.After(3 * time.Second):
					t.Fatal("cancellation did not reach helper cleanup")
				}
				lease, ok, err := r.f.service.deps.Engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, "data")
				require.NoError(t, err)
				if ok {
					lease.Release(t.Context())
				}
				require.False(t, ok, "admission must outlive unconfirmed helper removal")
				held.Store(false)
				if cause == "owner" {
					select {
					case err := <-drained:
						require.NoError(t, err)
					case <-time.After(3 * time.Second):
						t.Fatal("maintenance did not drain the download")
					}
					_, readErr := body.finish(t)
					require.Error(t, readErr)
				}
				r.requireReleased()
				require.Contains(t, []activitytypes.Status{activitytypes.StatusCancelled, activitytypes.StatusFailed}, r.activityStatus())
				_, _, removed := r.f.effects()
				require.Contains(t, removed, "scratch")
			})
		}
	}
}

// After an engine restart in the middle of a streamed download, startup repair settles the download activity as
// interrupted, startup recovery removes its labelled helper and scratch volume, and nothing replays the download.
func TestStreamedDownloadLeftoversAreRemovedByStartupRecovery(t *testing.T) {
	r := newStreamRig(t, "h1")
	r.mkdirHold = make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	body := readStream(r.get(ctx))
	select {
	case <-r.mkdirEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("restore phase did not start")
	}
	creates, scratchLabels, _ := r.f.effects()
	var helperLabels map[string]string
	for _, created := range creates {
		if created.Cmd[0] == "sleep" {
			helperLabels = created.Labels
		}
	}
	require.NotNil(t, helperLabels)
	require.NotNil(t, scratchLabels)
	scratch := scratchLabels[restore.RestoreLabel]
	// A new process has no tracked workers; its startup repair closes the abandoned download activity.
	restarted := activity.NewActivityService(r.f.service.deps.DB, nil)
	repaired, err := restarted.RepairStartup(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, repaired)
	var row activity.Activity
	require.NoError(t, r.f.service.deps.DB.Where("resource_type = ?", "volume_backup_download").First(&row).Error)
	require.Equal(t, activitytypes.StatusFailed, row.Status)
	require.NotNil(t, row.Error)
	require.Contains(t, *row.Error, "interrupted")
	removed := []string{}
	consumers := []string{}
	effects := []string{}
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(req.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]container.Summary{{ID: "download-helper", Labels: helperLabels, Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: scratch, Destination: "/scratch"}}}})
		case strings.HasSuffix(req.URL.Path, "/volumes"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Volumes": []map[string]any{{"Name": scratch, "Labels": scratchLabels}}})
		case req.Method == http.MethodDelete:
			removed = append(removed, req.URL.Path)
			w.WriteHeader(204)
		case strings.HasSuffix(req.URL.Path, "/json"):
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		default:
			effects = append(effects, req.Method+" "+req.URL.Path)
			w.WriteHeader(500)
		}
	}), &consumers)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.Len(t, removed, 2)
	require.Contains(t, removed[0], "download-helper")
	require.Contains(t, removed[1], scratch)
	require.Empty(t, consumers)
	require.Empty(t, effects, "startup recovery must not replay the download")
	var downloads int64
	require.NoError(t, r.f.service.deps.DB.Model(&activity.Activity{}).Where("resource_type = ?", "volume_backup_download").Count(&downloads).Error)
	require.EqualValues(t, 1, downloads)
	// The original process ends its own operation before the fixture goes away.
	cancel()
	_, _ = body.finish(t)
	r.requireReleased()
}
