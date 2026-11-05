// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original real registry/checker and stateful Docker image fixture for runtime transports.

package activity_test

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/google/go-containerregistry/pkg/name"
	registryHTTP "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

type containerImageHTTP struct {
	base                                            *recreateHTTP
	mu                                              sync.Mutex
	ref, repo, oldImage, newImage, oldManifest, tag string
	images                                          map[string]string
	creates, tags, pulls                            int
}

func newContainerImageHTTP(t *testing.T) (*containerImageHTTP, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(registryHTTP.New(registryHTTP.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	repo := strings.TrimPrefix(server.URL, "http://") + "/app"
	ref := repo + ":stable"
	makeImage := func(generation string) v1.Image {
		img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"generation": generation}}})
		require.NoError(t, err)
		return img
	}
	before, after := makeImage("before"), makeImage("after")
	oldID, err := before.ConfigName()
	require.NoError(t, err)
	newID, err := after.ConfigName()
	require.NoError(t, err)
	manifest, err := before.Digest()
	require.NoError(t, err)
	target, err := name.NewTag(ref, name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.Write(target, after, remote.WithContext(t.Context())))
	return &containerImageHTTP{base: newRecreateHTTP(t), ref: ref, repo: repo, oldImage: oldID.String(), newImage: newID.String(), oldManifest: manifest.String(), tag: oldID.String(), images: map[string]string{runtimeContainerID: oldID.String()}}, server
}
func (d *containerImageHTTP) serve(w http.ResponseWriter, r *http.Request) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := r.URL.Path
	if strings.HasPrefix(p, "/v") {
		p = p[strings.Index(p[1:], "/")+1:]
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case p == "/info":
		_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"amd64"}`))
		return true
	case p == "/images/create":
		d.pulls++
		d.tag = d.newImage
		_, _ = w.Write([]byte(`{"status":"pulled"}`))
		return true
	case strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/tag"):
		d.tags++
		d.tag = strings.TrimSuffix(strings.TrimPrefix(p, "/images/"), "/tag")
		w.WriteHeader(201)
		return true
	case strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/json"):
		ref := strings.TrimSuffix(strings.TrimPrefix(p, "/images/"), "/json")
		id := d.tag
		if strings.HasPrefix(ref, "sha256:") {
			id = ref
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "RepoDigests": []string{d.repo + "@" + d.oldManifest}, "Os": "linux", "Architecture": "amd64", "Config": map[string]any{}})
		return true
	case p == "/containers/json":
		d.base.mu.Lock()
		defer d.base.mu.Unlock()
		items := []map[string]any{}
		for id, item := range d.base.containers {
			items = append(items, map[string]any{"Id": id, "Names": []string{"/" + item.Name}, "Image": d.ref, "ImageID": d.images[id], "Labels": item.Labels, "State": "running"})
		}
		_ = json.NewEncoder(w).Encode(items)
		return true
	case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/json"):
		recorder := httptest.NewRecorder()
		if !d.base.serve(recorder, r) {
			return false
		}
		if recorder.Code != 200 {
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(recorder.Body.Bytes())
			return true
		}
		var body map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		body["Image"] = d.images[body["Id"].(string)]
		body["Config"].(map[string]any)["Image"] = d.ref
		_ = json.NewEncoder(w).Encode(body)
		return true
	case p == "/containers/create":
		var body struct {
			Image  string
			Labels map[string]string
			Env    []string
		}
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			http.Error(w, "invalid", 400)
			return true
		}
		d.creates++
		id := recreateNewID
		if d.creates > 1 {
			id = strings.Repeat("e", 64)
		}
		d.base.mu.Lock()
		d.base.effects["create"]++
		d.base.containers[id] = recreateObserved{ID: id, Name: r.URL.Query().Get("name"), Labels: body.Labels, Environment: body.Env, Created: time.Now().UTC().Format(time.RFC3339Nano)}
		d.base.mu.Unlock()
		d.images[id] = d.tag
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
		return true
	}
	return d.base.serve(w, r)
}
func configureContainerImageFixture(t *testing.T, f *runtimeServicesFixture, d *containerImageHTTP, regServer *httptest.Server) {
	t.Helper()
	f.http.onCall = d.serve
	require.NoError(t, f.db.Create(&registry.ContainerRegistry{URL: regServer.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
	reg := registry.NewContainerRegistryService(f.db, nil, nil, f.settings, regServer.Client())
	checks := imageupdate.NewImageUpdateService(f.db, f.settings, reg, f.docker, nil, nil, nil)
	events := event.NewEventService(f.db, nil, regServer.Client())
	// Preserve the pointer already held by ContainerService; configure before execution.
	*f.images = *image.NewImageService(f.db, f.docker, reg, checks, nil, events, f.settings, f.activity)
	runtime := francistest.New(t)
	gate := runs.NewAdmission(runtime.Service(), t.Name())
	require.NoError(t, gate.Register(runtime))
	francistest.Start(t, runtime)
	f.images.SetImageAdmission(gate, nil)
	f.containers.ConfigureOperations(f.db.DB, "example-registration", false, nil, nil)
}
