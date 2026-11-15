// Source: docklet-next/issues/412
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original per-reference producer, dashboard and Go-to-web fixture evidence.

package image_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	containerService "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/dashboard"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	imageService "github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	dashboardDTO "github.com/florianfinn/docklet-next/types/dashboard"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/google/go-containerregistry/pkg/name"
	registryServer "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type referenceFixture struct {
	db         *database.DB
	docker     *docker.DockerClientService
	containers []container.Summary
	refs       []string
	denied     atomic.Bool
	manifests  [2]atomic.Int32
	tagLists   atomic.Int32
}

func newReferenceFixture(t *testing.T) *referenceFixture {
	t.Helper()
	f := &referenceFixture{}
	handler := registryServer.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tags/list") {
			f.tagLists.Add(1)
		}
		if f.denied.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		for i, tag := range []string{"stable", "edge"} {
			if r.Method == http.MethodHead && r.URL.Path == "/v2/app/manifests/"+tag {
				f.manifests[i].Add(1)
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	cfg, err := empty.Image.ConfigFile()
	require.NoError(t, err)
	cfg.OS, cfg.Architecture = "linux", "amd64"
	cfg.Config.Labels = map[string]string{"org.opencontainers.image.version": "1.0.0"}
	original, err := mutate.ConfigFile(empty.Image, cfg)
	require.NoError(t, err)
	oldID, err := original.ConfigName()
	require.NoError(t, err)
	oldManifest, err := original.Digest()
	require.NoError(t, err)
	nextConfig := cfg.DeepCopy()
	nextConfig.Config.Labels = map[string]string{"org.opencontainers.image.version": "1.1.0"}
	next, err := mutate.ConfigFile(original, nextConfig)
	require.NoError(t, err)
	for _, tag := range []string{"stable", "edge"} {
		ref := host + "/app:" + tag
		parsed, err := name.NewTag(ref, name.Insecure)
		require.NoError(t, err)
		require.NoError(t, remote.Write(parsed, next, remote.WithContext(t.Context())))
		f.refs = append(f.refs, ref)
	}
	for i := range f.manifests {
		f.manifests[i].Store(0)
	}
	for i, id := range []string{"first", "second", "replica"} {
		ref := f.refs[0]
		if i == 1 {
			ref = f.refs[1]
		}
		f.containers = append(f.containers, container.Summary{ID: id, Names: []string{"/" + id}, Image: ref, ImageID: oldID.String(), State: "running", Status: "Up", Labels: map[string]string{}})
	}
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			require.NoError(t, json.NewEncoder(w).Encode(f.containers))
		case strings.Contains(r.URL.Path, "/containers/"):
			for _, cnt := range f.containers {
				if strings.HasSuffix(r.URL.Path, "/containers/"+cnt.ID+"/json") {
					require.NoError(t, json.NewEncoder(w).Encode(container.InspectResponse{ID: cnt.ID, Image: cnt.ImageID, Config: &container.Config{Image: cnt.Image, Labels: cnt.Labels}}))
					return
				}
			}
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/images/json"):
			// Duplicate inventory rows deduplicate; edge is found from its live container.
			require.NoError(t, json.NewEncoder(w).Encode([]image.Summary{{ID: oldID.String(), RepoTags: []string{f.refs[0]}}, {ID: oldID.String(), RepoTags: []string{f.refs[0]}}}))
		case strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"amd64"}`))
		case strings.HasSuffix(r.URL.Path, "/networks"):
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			_, _ = w.Write([]byte(`{"Volumes":[],"Warnings":[]}`))
		case strings.Contains(r.URL.Path, "/images/"):
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Id": oldID.String(), "RepoDigests": []string{host + "/app@" + oldManifest.String()}, "Os": "linux", "Architecture": "amd64", "Config": map[string]any{"Labels": map[string]string{"org.opencontainers.image.version": "1.0.0"}}}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(daemon.Close)
	cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cli.Close()) })
	f.docker = docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sql.Close()) })
	require.NoError(t, db.AutoMigrate(&imageupdate.ImageUpdateRecord{}, &registry.ContainerRegistry{}, &apikey.ApiKey{}))
	f.db = &database.DB{DB: db}
	require.NoError(t, db.Create(&registry.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
	return f
}

func (f *referenceFixture) checker() *imageupdate.ImageUpdateService {
	return imageupdate.NewImageUpdateService(f.db, nil, registry.NewContainerRegistryService(f.db, nil, nil, nil), f.docker, nil, nil, nil)
}
func (f *referenceFixture) reader() *imageService.ImageService {
	return imageService.NewImageService(f.db, f.docker, nil, nil, nil, nil, nil, nil)
}

func (f *referenceFixture) assertRows(t *testing.T) {
	t.Helper()
	for _, cnt := range f.containers {
		var row imageupdate.ImageUpdateRecord
		require.NoError(t, f.db.First(&row, "id = ?", "container::"+cnt.ID).Error)
		require.Equal(t, cnt.ID, row.ContainerID)
		require.Equal(t, cnt.ImageID, row.ImageID)
		require.Equal(t, imageref.UpdatePolicyKey(cnt.Image, cnt.Labels), row.PolicyKey)
		require.True(t, imageupdate.SameUpdateReference(row.Repository+":"+row.Tag, cnt.Image))
		require.True(t, row.UpdateInfo().HasUpdate)
	}
	var n int64
	require.NoError(t, f.db.Model(&imageupdate.ImageUpdateRecord{}).Where("container_id = ''").Count(&n).Error)
	require.EqualValues(t, 1, n, "the image-level writer keeps its single image-ID key")
}

func TestPerReferenceDigestProducers(t *testing.T) {
	for _, mode := range []string{"single", "batch", "single-then-batch", "explicit-batch"} {
		t.Run(mode, func(t *testing.T) {
			f := newReferenceFixture(t)
			if mode == "single" || mode == "single-then-batch" {
				for _, ref := range f.refs {
					result, err := f.checker().ReadUpdateInfo(t.Context(), ref)
					require.NoError(t, err)
					require.True(t, result.HasUpdate)
				}
				f.assertRows(t)
			}
			if mode != "single" {
				var result map[string]*checkDTO.Response
				var err error
				if mode == "explicit-batch" {
					result, err = f.checker().ReadMultiple(t.Context(), append(append([]string{}, f.refs...), f.refs[0]))
				} else {
					result, err = f.checker().ReadAll(t.Context())
				}
				require.NoError(t, err)
				require.Len(t, result, 2)
				f.assertRows(t)
			}
			updates, err := f.reader().GetUpdateInfoByContainers(t.Context(), f.containers)
			require.NoError(t, err)
			require.Len(t, updates, 3)
			for _, cnt := range f.containers {
				require.True(t, updates[cnt.ID].HasUpdate)
				require.True(t, imageupdate.SameUpdateReference(updates[cnt.ID].ImageRef, cnt.Image))
			}
			want := int32(1)
			if mode == "single-then-batch" {
				want = 2
			}
			for i := range f.manifests {
				require.Equal(t, want, f.manifests[i].Load(), "one registry reference lookup per check, shared by replicas")
			}
		})
	}
}

func TestPerReferenceTagBatchPreservesDigestRows(t *testing.T) {
	f := newReferenceFixture(t)
	for _, ref := range f.refs {
		_, err := f.checker().ReadUpdateInfo(t.Context(), ref)
		require.NoError(t, err)
	}
	f.assertRows(t)
	require.NoError(t, f.db.Exec("CREATE TABLE projects (id TEXT PRIMARY KEY, build_image_refs_json TEXT)").Error)
	require.NoError(t, f.db.Exec("CREATE TABLE image_patches (status TEXT, patched_ref TEXT)").Error)
	// This legacy batch's tag-policy phase must not erase native digest rows.
	// Image-level results may fail independently; container evidence stays bound.
	_, err := f.checker().CheckMultipleImages(t.Context(), f.refs, nil)
	require.NoError(t, err)
	updates, err := f.reader().GetUpdateInfoByContainers(t.Context(), f.containers)
	require.NoError(t, err)
	require.Len(t, updates, 3)
	for _, cnt := range f.containers {
		require.True(t, updates[cnt.ID].HasUpdate)
	}
	f.assertRows(t)
}

func TestPerReferenceFailedRecheckReplacesSuccess(t *testing.T) {
	f := newReferenceFixture(t)
	_, err := f.checker().ReadAll(t.Context())
	require.NoError(t, err)
	f.assertRows(t)
	f.denied.Store(true)
	result, err := f.checker().ReadUpdateInfo(t.Context(), f.refs[0])
	require.NoError(t, err)
	require.Equal(t, "error", result.State)
	updates, err := f.reader().GetUpdateInfoByContainers(t.Context(), f.containers)
	require.NoError(t, err)
	for _, id := range []string{"first", "replica"} {
		require.Equal(t, "error", updates[id].State)
		require.False(t, updates[id].HasUpdate)
	}
	require.True(t, updates["second"].HasUpdate, "another reference retains its result")
}

func TestPerReferenceInitialFailureHasScopedRows(t *testing.T) {
	f := newReferenceFixture(t)
	f.denied.Store(true)
	results, err := f.checker().ReadAll(t.Context())
	require.NoError(t, err)
	for _, result := range results {
		require.Equal(t, "error", result.State)
	}
	updates, err := f.reader().GetUpdateInfoByContainers(t.Context(), f.containers)
	require.NoError(t, err)
	require.Len(t, updates, 3)
	for _, cnt := range f.containers {
		require.Equal(t, "error", updates[cnt.ID].State)
		require.False(t, updates[cnt.ID].HasUpdate)
		require.True(t, imageupdate.SameUpdateReference(cnt.Image, updates[cnt.ID].ImageRef))
	}
	require.EqualValues(t, 0, f.tagLists.Load(), "a failed image check never starts tag discovery")
}

func TestPerReferenceFailedImageDoesNotRetryTagDiscovery(t *testing.T) {
	f := newReferenceFixture(t)
	f.containers[0].Labels = map[string]string{"com.getarcaneapp.arcane.updater.strategy": "tag"}
	f.denied.Store(true)
	result, err := f.checker().ReadUpdateInfo(t.Context(), f.refs[0])
	require.NoError(t, err)
	require.Equal(t, "error", result.State)
	require.Nil(t, result.ContainerUpdates, "preserve the original failed image response for tag-policy consumers")
	require.EqualValues(t, 0, f.tagLists.Load(), "a failed image check never starts tag discovery")
}

func TestPerReferenceDashboardCounts(t *testing.T) {
	f := newReferenceFixture(t)
	_, err := f.checker().ReadAll(t.Context())
	require.NoError(t, err)
	// Exclude the replica from this two-container count without changing its observation.
	f.containers = f.containers[:2]
	for _, want := range []int{2, 1} {
		if want == 1 {
			require.NoError(t, f.db.Model(&imageupdate.ImageUpdateRecord{}).Where("container_id = ?", "second").Update("latest_digest", f.containers[1].ImageID).Error)
		}
		svc := dashboard.NewDashboardService(f.db, f.docker, nil, nil, f.reader(), nil, nil, nil, nil, nil)
		snapshot, err := svc.GetSnapshot(t.Context(), dashboard.DashboardActionItemsOptions{}, false)
		require.NoError(t, err)
		found := false
		for _, item := range snapshot.ActionItems.Items {
			if item.Kind == dashboardDTO.ActionItemKindImageUpdates {
				require.Equal(t, want, item.Count)
				found = true
			}
		}
		require.True(t, found, "actual dashboard pending-update action must be present")
	}
}

// UPDATE_WEB_FIXTURES=1 regenerates the exact Go producer/reader/DTO bytes.
func TestPerReferenceWebFixture(t *testing.T) {
	f := newReferenceFixture(t)
	_, err := f.checker().ReadAll(t.Context())
	require.NoError(t, err)
	updates, err := f.reader().GetUpdateInfoByContainers(t.Context(), f.containers)
	require.NoError(t, err)
	rows := containerService.NewContainerService(nil, f.docker, f.reader(), nil, nil).BuildSummaries(t.Context(), f.containers[:2], updates, "", nil)
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	for i := range rows {
		cnt := &rows[i]
		require.NotNil(t, cnt.UpdateInfo)
		require.True(t, cnt.UpdateInfo.HasUpdate)
		tag := []string{"stable", "edge"}[i]
		cnt.Image = "registry.example.com/app:" + tag
		cnt.UpdateInfo.ImageRef = cnt.Image
		cnt.UpdateInfo.CheckTime, cnt.UpdateInfo.CheckedAt = at, at
	}
	writeReferenceWebFixture(t, rows)
}
func writeReferenceWebFixture(t *testing.T, rows []containerDTO.Summary) {
	t.Helper()
	body, err := json.MarshalIndent(rows, "", "  ")
	require.NoError(t, err)
	body = append(body, '\n')
	path := "../../../web/src/data/http/testdata/per-reference-updates.json"
	if os.Getenv("UPDATE_WEB_FIXTURES") == "1" {
		require.NoError(t, os.WriteFile(path, body, 0644))
	}
	expected, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(expected, body), "web fixture differs from producer bytes; regenerate with UPDATE_WEB_FIXTURES=1")
}
