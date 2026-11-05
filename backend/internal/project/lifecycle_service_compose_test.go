// Source: docklet-next/issues/343
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original real Compose SDK to stateful Docker HTTP double scope proof.

package project

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"github.com/moby/moby/api/types/registry"
	"github.com/stretchr/testify/require"
)

type serviceComposeDocker struct {
	f         *lifecycleFixture
	failStart bool
	labels    map[string]map[string]string
	names     map[string]string
	calls     []string
}

func (d *serviceComposeDocker) serve(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if len(path) > 2 && path[1] == 'v' {
		if i := strings.Index(path[1:], "/"); i >= 0 {
			path = path[i+1:]
		}
	}
	f := d.f
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	labels := func(c lifecycleWorkload) map[string]string {
		if values := d.labels[c.ID]; values != nil {
			return values
		}
		values := c.labels()
		values[api.ContainerNumberLabel] = "1"
		values[api.OneoffLabel] = "False"
		return values
	}
	switch {
	case path == "/_ping":
		w.Header().Set("API-Version", "1.48")
		_, _ = fmt.Fprint(w, "OK")
	case path == "/version":
		write(map[string]any{"ApiVersion": "1.48", "Version": "28.0.0", "Os": "linux", "Arch": "amd64"})
	case path == "/info":
		write(map[string]any{"OSType": "linux", "Architecture": "x86_64"})
	case path == "/networks":
		write([]any{})
	case path == "/volumes":
		write(map[string]any{"Volumes": []any{}, "Warnings": []any{}})
	case path == "/containers/json":
		rows := []map[string]any{}
		var filters map[string][]string
		_ = json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters)
		for _, c := range f.workloads {
			matched := true
			for _, filter := range filters["label"] {
				key, value, equal := strings.Cut(filter, "=")
				actual, exists := labels(c)[key]
				if !exists || equal && actual != value {
					matched = false
				}
			}
			if !matched {
				continue
			}
			state := "exited"
			if c.Running {
				state = "running"
			}
			name := d.names[c.ID]
			if name == "" {
				name = "/example-stack-" + c.Service + "-1"
			}
			rows = append(rows, map[string]any{"Id": c.ID, "Names": []string{name}, "Image": c.Reference, "ImageID": c.ImageID, "State": state, "Labels": labels(c), "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
		}
		write(rows)
	case path == "/containers/create":
		var body struct {
			Image  string
			Labels map[string]string
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Error(err)
			w.WriteHeader(500)
			return
		}
		service := body.Labels[api.ServiceLabel]
		d.calls = append(d.calls, "create:"+service)
		id := strings.Repeat("3", 64)
		image, ok := f.imageID(body.Image)
		if !ok {
			f.t.Errorf("unknown create image %s", body.Image)
			w.WriteHeader(404)
			return
		}
		f.workloads[id] = lifecycleWorkload{ID: id, Service: service, Project: "example-stack", Reference: body.Image, ImageID: image, ConfigHash: body.Labels[api.ConfigHashLabel]}
		d.labels[id] = body.Labels
		d.names[id] = "/" + r.URL.Query().Get("name")
		w.WriteHeader(201)
		write(map[string]any{"Id": id, "Warnings": []string{}})
	case strings.HasPrefix(path, "/containers/"):
		rest := strings.TrimPrefix(path, "/containers/")
		id, action, _ := strings.Cut(rest, "/")
		for key, c := range f.workloads {
			if c.ID != id {
				continue
			}
			switch action {
			case "json":
				status := "exited"
				if c.Running {
					status = "running"
				}
				write(map[string]any{"Id": c.ID, "Name": d.names[c.ID], "Image": c.ImageID, "Config": map[string]any{"Image": c.Reference, "Labels": labels(c)}, "HostConfig": map[string]any{"NetworkMode": "none"}, "State": map[string]any{"Status": status, "Running": c.Running, "ExitCode": 0}, "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
			case "rename":
				d.names[id] = "/" + r.URL.Query().Get("name")
				d.calls = append(d.calls, "rename:"+c.Service)
				w.WriteHeader(204)
			case "stop":
				c.Running = false
				f.workloads[key] = c
				d.calls = append(d.calls, "stop:"+c.Service)
				w.WriteHeader(204)
			case "start":
				d.calls = append(d.calls, "start:"+c.Service)
				if d.failStart && c.Service == "app" {
					w.WriteHeader(500)
					write(map[string]string{"message": "synthetic selected start failure"})
					return
				}
				c.Running = true
				f.workloads[key] = c
				w.WriteHeader(204)
			case "":
				if r.Method != http.MethodDelete {
					f.t.Errorf("unexpected container method %s", r.Method)
					w.WriteHeader(500)
					return
				}
				d.calls = append(d.calls, "remove:"+c.Service)
				delete(f.workloads, key)
				w.WriteHeader(204)
			default:
				f.t.Errorf("unexpected container action %s", action)
				w.WriteHeader(500)
			}
			return
		}
		w.WriteHeader(404)
		write(map[string]string{"message": "container not found"})
	default:
		// Image operations use the same state as lifecycle checks, pulls and pins.
		f.mu.Unlock()
		f.serve(w, r)
		f.mu.Lock()
	}
}

func TestServiceScopeRealComposeKeepsUnselectedContainers(t *testing.T) {
	for _, outcome := range []string{"success", "start-failure"} {
		t.Run(outcome, func(t *testing.T) {
			f := serviceScopeFixture(t, serviceScopeSource)
			require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, ".env"), []byte("COMPOSE_REMOVE_ORPHANS=true\n"), 0600))
			f.establishBaseline()
			before := map[string]lifecycleWorkload{"running": f.workloads["running"], "stopped": f.workloads["stopped"]}
			d := &serviceComposeDocker{f: f, failStart: outcome == "start-failure", labels: map[string]map[string]string{}, names: map[string]string{}}
			server := httptest.NewServer(http.HandlerFunc(d.serve))
			t.Cleanup(server.Close)
			t.Setenv("DOCKER_HOST", strings.Replace(server.URL, "http://", "tcp://", 1))
			t.Setenv("DOCKER_API_VERSION", "1.48")
			t.Setenv("DOCKER_CONFIG", t.TempDir())
			t.Setenv("COMPOSE_REMOVE_ORPHANS", "true")
			f.service.composeCoordinator = projects.NewCoordinator(projectDTO.ComposeCommands{Up: func(ctx context.Context, model *compose.Project, services []string, remove, force, volumes bool, auth map[string]registry.AuthConfig, wait time.Duration) error {
				require.Len(t, model.Services, 1)
				require.True(t, force)
				require.False(t, remove)
				// The environment survives the model loader and reaches the real merge.
				require.Equal(t, "true", model.Environment["COMPOSE_REMOVE_ORPHANS"])
				return projects.ComposeUp(ctx, model, services, remove, force, volumes, auth, wait)
			}})
			intent := f.intent("version")
			intent.Request.Services = []string{"app"}
			out := f.wait(f.admit(intent))
			if outcome == "success" {
				require.Equal(t, activityDTO.StatusSuccess, out.Status, "%+v", out.ImageLifecycle)
				require.True(t, out.ImageLifecycle.Verified)
			} else {
				require.Equal(t, activityDTO.StatusFailed, out.Status)
				require.Equal(t, "update.deploy-failed", out.ImageLifecycle.Code)
			}
			require.NotEmpty(t, out.ImageLifecycle.RestorePointID)
			points, err := f.service.ListImageRestorePoints(t.Context(), f.project.ID, false)
			require.NoError(t, err)
			require.Len(t, points, 1)
			require.True(t, points[0].Available)
			require.Equal(t, "settled", points[0].SourceState)
			require.False(t, points[0].FailedAttempt, "a healthy source remains a healthy recovery point")
			if outcome == "success" {
				_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
				require.NoError(t, err)
				baseline, _, err := f.service.lifecycleStore().Baseline(t.Context(), current)
				require.NoError(t, err)
				require.Len(t, baseline.Services, 3)
				require.Equal(t, lifecycleNewID, baseline.Services["app"].ImageID)
				require.NotEqual(t, lifecycleContainerID, baseline.Services["app"].ContainerID)
				require.True(t, baseline.Services["running"].Running)
				require.False(t, baseline.Services["stopped"].Running)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Equal(t, before["running"], f.workloads["running"])
			require.Equal(t, before["stopped"], f.workloads["stopped"])
			require.Empty(t, f.stopSignals, "pre-Up custom stop must exclude running")
			require.Contains(t, d.calls, "create:app")
			require.Contains(t, d.calls, "start:app")
			for _, call := range d.calls {
				require.True(t, strings.HasSuffix(call, ":app"), "unselected Docker effect: %s", call)
			}
		})
	}
}
