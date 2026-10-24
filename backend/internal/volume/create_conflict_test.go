// Source: docklet-next/issues/221
// Revision: 75e73d5c4ffcd78b47ebaa166579079c02f70c8f
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original API create conflict, internal reuse and Compose reuse regressions.

package volume

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type createDockerFixture struct {
	volumes       map[string]volume.Volume
	inspectStatus int
	creates       int
	reads         int
}

func (f *createDockerFixture) server(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "HEAD" && r.URL.Path == "/_ping":
			w.Header().Set("API-Version", "1.53")
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/volumes"):
			f.reads++
			rows := []volume.Volume{}
			for _, v := range f.volumes {
				rows = append(rows, v)
			}
			_ = json.NewEncoder(w).Encode(volume.ListResponse{Volumes: rows})
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/volumes/"):
			f.reads++
			if f.inspectStatus != 0 {
				w.WriteHeader(f.inspectStatus)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "synthetic inspect failure"})
				return
			}
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if v, ok := f.volumes[name]; ok {
				_ = json.NewEncoder(w).Encode(v)
				return
			}
			w.WriteHeader(404)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "no such volume"})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/volumes/create"):
			f.creates++
			var options client.VolumeCreateOptions
			require.NoError(t, json.NewDecoder(r.Body).Decode(&options))
			v, ok := f.volumes[options.Name]
			if !ok {
				v = pruneVolume(options.Name)
				v.Labels = options.Labels
				v.Options = options.DriverOpts
				f.volumes[v.Name] = v
			}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(v)
		case r.Method == "GET" && (strings.HasSuffix(r.URL.Path, "/containers/json") || strings.HasSuffix(r.URL.Path, "/networks")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			_, _ = w.Write([]byte(`{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Os":"linux","Architecture":"amd64","Config":{}}`))
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"x86_64"}`))
		default:
			t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func (f *createDockerFixture) service(t *testing.T) *VolumeService {
	t.Helper()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&event.Event{}))
	return &VolumeService{dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, f.server(t))), eventService: event.NewEventService(&database.DB{DB: db}, nil, nil)}
}

func createRouter(t *testing.T, svc *VolumeService) http.Handler {
	t.Helper()
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			grants := authz.NewPermissionSet()
			grants.AddEnv("0", authz.PermVolumesCreate)
			ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, grants)
			ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "example-user", Username: "example-user"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("storage", "test"))
	RegisterVolumes(api, nil, svc, nil, nil, handlerutil.NewActivityAppContext(t.Context()))
	return router
}

func TestCreateVolumeThroughDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, scenario := range []string{"existing-local", "existing-plugin", "new", "inspect-failed"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				f := &createDockerFixture{volumes: map[string]volume.Volume{}}
				if strings.HasPrefix(scenario, "existing") {
					v := pruneVolume("occupied")
					v.Labels = map[string]string{"owner": "original"}
					if scenario == "existing-plugin" {
						v.Driver = "plugin"
					}
					f.volumes[v.Name] = v
				}
				if scenario == "inspect-failed" {
					f.inspectStatus = 500
				}
				call := pruneTransport(t, mode, createRouter(t, f.service(t)))
				status, body := call("POST", "/api/environments/0/volumes", "", []byte(`{"name":"occupied","driver":"local","labels":{"owner":"new"}}`))
				switch scenario {
				case "new":
					require.Equal(t, 200, status, string(body))
					var response base.ApiResponse[volumetypes.Volume]
					require.NoError(t, json.Unmarshal(body, &response))
					require.True(t, response.Success)
					require.Equal(t, "occupied", response.Data.Name)
					require.Equal(t, 1, f.creates)
				case "inspect-failed":
					require.Equal(t, 500, status, string(body))
					require.Zero(t, f.creates)
					require.Empty(t, f.volumes)
				default:
					require.Equal(t, 409, status, string(body))
					var problem volumetypes.ExistsError
					require.NoError(t, json.Unmarshal(body, &problem))
					require.Equal(t, volumetypes.ExistsError{Status: 409, Title: "Conflict", Detail: "A volume with this name already exists", Code: "volume.exists"}, problem)
					require.Zero(t, f.creates)
					require.Equal(t, map[string]string{"owner": "original"}, f.volumes["occupied"].Labels)
				}
			})
		}
	}
}

func TestInternalCreateVolumeStillReusesExistingVolume(t *testing.T) {
	existing := pruneVolume("occupied")
	existing.Labels = map[string]string{"owner": "original"}
	f := &createDockerFixture{volumes: map[string]volume.Volume{existing.Name: existing}}
	response, err := f.service(t).CreateVolume(t.Context(), client.VolumeCreateOptions{Name: existing.Name, Driver: "local", Labels: map[string]string{"owner": "new"}}, user.Actor{ID: "example-user", Username: "example-user"})
	require.NoError(t, err)
	require.Equal(t, "occupied", response.Name)
	require.Equal(t, existing.Labels, response.Labels)
	require.Equal(t, 1, f.creates)
	require.Len(t, f.volumes, 1)
}

func TestComposeCreateStillReusesExistingVolume(t *testing.T) {
	existing := pruneVolume("example_data")
	f := &createDockerFixture{volumes: map[string]volume.Volume{existing.Name: existing}}
	server := f.server(t)
	t.Setenv("DOCKER_HOST", strings.Replace(server.URL, "http://", "tcp://", 1))
	t.Setenv("DOCKER_API_VERSION", "1.53")
	zero := 0
	model := &composetypes.Project{Name: "example", Services: composetypes.Services{"app": {Name: "app", Image: "example:1.0", Scale: &zero, Volumes: []composetypes.ServiceVolumeConfig{{Type: "volume", Source: "data", Target: "/data"}}}}, Volumes: composetypes.Volumes{"data": {Name: existing.Name, Driver: "local"}}}
	require.NoError(t, projects.ComposeCreate(t.Context(), model, nil, nil))
	require.Contains(t, model.Volumes, "data", "the deployed service references this volume")
	require.Positive(t, f.reads, "Compose must observe the existing volume")
	require.Zero(t, f.creates, "Compose reuses the existing volume without replacing it")
	require.Equal(t, existing, f.volumes[existing.Name])
	require.Len(t, f.volumes, 1)
}
