// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original stateful image/container Docker producer and real slot/guard fixture. M24 #440: engine start/AutoRemove semantics, errors after completed effects.

package container

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	dc "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

const updateOldID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const updateRestoreID = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
const updateNewID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const updateOldImage = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
const updateNewImage = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
const updateOtherImage = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
const updateRef = "registry.example.com/app:stable"

type updateDaemon struct {
	mu                    sync.Mutex
	containers            map[string]dc.InspectResponse
	tag                   string
	counts                map[string]int
	fail                  string
	moved                 bool
	pullRef, pullPlatform string
	createSlot            func()
	beforeRefInspect      func()
	// containerd maps an image ID (the index) to its platform manifest; set,
	// inspects answer in the containerd image store's shape.
	containerd map[string]string
	// inspect adjusts an exact container inspect answer; failAfter applies the
	// named effect, then answers with an error (the daemon completed it).
	inspect   func(id string, item *dc.InspectResponse)
	failAfter string
}

func newUpdateDaemon() *updateDaemon {
	return &updateDaemon{containers: map[string]dc.InspectResponse{updateOldID: {ID: updateOldID, Name: "/app", Image: updateOldImage, HostConfig: &dc.HostConfig{Binds: []string{"data:/data"}}, State: &dc.State{Running: true}, Config: &dc.Config{Image: updateRef, Labels: map[string]string{}, Env: []string{"EXAMPLE=kept"}}}}, tag: updateOldImage, counts: map[string]int{}}
}
func (d *updateDaemon) serve(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := r.URL.Path
	if strings.HasPrefix(p, "/v") {
		p = p[strings.Index(p[1:], "/")+1:]
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case p == "/version":
		_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.41"})
	case p == "/info":
		_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"amd64"}`))
	case p == "/images/create":
		d.counts["pull"]++
		d.pullRef = r.URL.Query().Get("fromImage")
		d.pullPlatform = r.URL.Query().Get("platform")
		if d.fail == "pull" {
			http.Error(w, "synthetic pull failure", 500)
			return
		}
		d.tag = updateNewImage
		if d.moved {
			d.tag = updateOtherImage
		}
		_, _ = w.Write([]byte(`{"status":"pulled"}`))
	case strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/tag"):
		d.counts["tag"]++
		d.tag = strings.TrimSuffix(strings.TrimPrefix(p, "/images/"), "/tag")
		w.WriteHeader(201)
	case strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/json") && p != "/images/json":
		ref := strings.TrimSuffix(strings.TrimPrefix(p, "/images/"), "/json")
		if d.beforeRefInspect != nil && !strings.HasPrefix(ref, "sha256:") {
			callback := d.beforeRefInspect
			d.beforeRefInspect = nil
			callback()
		}
		id := d.tag
		if strings.HasPrefix(ref, "sha256:") {
			id = ref
		}
		if manifest, ok := d.containerd[id]; ok {
			descriptor := map[string]any{"mediaType": "application/vnd.oci.image.index.v1+json", "digest": id, "size": 1}
			if r.URL.Query().Has("platform") {
				id, descriptor = manifest, map[string]any{"mediaType": "application/vnd.oci.image.manifest.v1+json", "digest": manifest, "size": 1}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Descriptor": descriptor, "Os": "linux", "Architecture": "amd64", "Config": map[string]any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Os": "linux", "Architecture": "amd64", "Config": map[string]any{}})
	case p == "/images/json":
		_ = json.NewEncoder(w).Encode([]map[string]any{{"Id": updateOldImage, "Created": 100}})
	case strings.HasPrefix(p, "/images/") && r.Method == http.MethodDelete:
		d.counts["delete-image"]++
		_, _ = w.Write([]byte(`[]`))
	case p == "/containers/json":
		items := []map[string]any{}
		for id, item := range d.containers {
			items = append(items, map[string]any{"Id": id, "ImageID": item.Image, "Labels": item.Config.Labels})
		}
		_ = json.NewEncoder(w).Encode(items)
	case p == "/containers/create":
		d.counts["create"]++
		if d.createSlot != nil {
			d.createSlot()
		}
		if d.fail == "create" {
			http.Error(w, "synthetic create failure", 500)
			return
		}
		var body struct {
			dc.Config
			HostConfig dc.HostConfig
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid", 400)
			return
		}
		createdID := updateNewID
		if d.counts["create"] > 1 {
			createdID = updateRestoreID
		}
		d.containers[createdID] = dc.InspectResponse{ID: createdID, Name: "/" + r.URL.Query().Get("name"), Image: d.tag, HostConfig: &body.HostConfig, State: &dc.State{}, Created: time.Now().Format(time.RFC3339Nano), Config: &body.Config}
		if d.failAfter == "create" {
			d.failAfter = ""
			http.Error(w, "synthetic error after effect", 500)
			return
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": createdID})
	case strings.HasPrefix(p, "/containers/"):
		parts := strings.Split(strings.TrimPrefix(p, "/containers/"), "/")
		id := parts[0]
		if _, ok := d.containers[id]; !ok {
			for k, v := range d.containers {
				if strings.TrimPrefix(v.Name, "/") == id {
					id = k
					break
				}
			}
		}
		item, ok := d.containers[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "json" {
			if d.inspect != nil {
				d.inspect(id, &item)
			}
			_ = json.NewEncoder(w).Encode(item)
			return
		}
		op := "remove"
		if len(parts) > 1 {
			op = parts[1]
		}
		d.counts[op]++
		if d.fail == op && (op != "start" || id == updateNewID) {
			http.Error(w, "synthetic effect failure", 500)
			return
		}
		switch op {
		case "rename":
			item.Name = "/" + r.URL.Query().Get("name")
		case "stop":
			item.State.Running = false
			if item.HostConfig != nil && item.HostConfig.AutoRemove {
				delete(d.containers, id)
				w.WriteHeader(204)
				return
			}
		case "start":
			// An API start resets the restart counter, as the engine does.
			item.State.Running, item.State.StartedAt, item.RestartCount = true, time.Now().UTC().Format(time.RFC3339Nano), 0
		case "remove":
			delete(d.containers, id)
			if d.failAfter == op {
				d.failAfter = ""
				http.Error(w, "synthetic error after effect", 500)
				return
			}
			w.WriteHeader(204)
			return
		default:
			http.Error(w, "unexpected operation", 500)
			return
		}
		d.containers[id] = item
		if d.failAfter == op {
			d.failAfter = ""
			http.Error(w, "synthetic error after effect", 500)
			return
		}
		w.WriteHeader(204)
	default:
		http.NotFound(w, r)
	}
}

type updateImagePort struct {
	g        *guard.Service
	checked  *checkDTO.Response
	check    func()
	expected string
}

func (p *updateImagePort) CheckContainerImage(context.Context, dc.InspectResponse) (*checkDTO.Response, error) {
	if p.check != nil {
		p.check()
	}
	return p.checked, nil
}
func (p *updateImagePort) ExpectedContainerImage(context.Context, dc.InspectResponse, string) (string, string, error) {
	return p.expected, "linux/amd64", nil
}
func (p *updateImagePort) ImageGuard() *guard.Service { return p.g }
func (p *updateImagePort) PullOptionsWithAuth(context.Context, string, []containerregistry.Credential) (client.ImagePullOptions, error) {
	return client.ImagePullOptions{}, nil
}

func updateFixture(t *testing.T) (*ContainerService, *updateDaemon, *updateImagePort, *database.DB) {
	t.Helper()
	return updateFixtureAPI(t, "1.41")
}
func updateFixtureAPI(t *testing.T, api string) (*ContainerService, *updateDaemon, *updateImagePort, *database.DB) {
	t.Helper()
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "update.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	d := newUpdateDaemon()
	server := httptest.NewServer(http.HandlerFunc(d.serve))
	t.Cleanup(server.Close)
	c, err := client.New(client.WithHost(server.URL), client.WithAPIVersion(api))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	f := francistest.New(t)
	gate := runs.NewAdmission(f.Service(), t.Name())
	require.NoError(t, gate.Register(f))
	francistest.Start(t, f)
	g := &guard.Service{DB: db, Admission: gate, Docker: func(context.Context) (*client.Client, error) { return c, nil }}
	port := &updateImagePort{g: g, checked: &checkDTO.Response{HasUpdate: true, State: "rebuild", LatestDigest: updateNewImage}, expected: updateNewImage}
	s := &ContainerService{dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(c), containerImages: port}
	s.ConfigureOperations(db.DB, "registration", true, nil, nil)
	return s, d, port, db
}
func updateContext(t *testing.T) context.Context {
	return updaterDTO.WithExecutionIdentity(t.Context(), updaterDTO.ExecutionIdentity{RunID: uuid.NewString(), RegistrationID: "registration"})
}
