// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/stretchr/testify/require"
)

// Engine receiver records the real embedded Compose sender. It implements only
// the read/recreate API used by this one-service package profile, never Docker.
type composeEngine struct {
	mu                           sync.Mutex
	request                      Request
	oldRemoved, created, started bool
	newConfig                    map[string]any
	mutations                    []string
	images                       []string
	reads                        []string
}

func (e *composeEngine) labels() map[string]string {
	return map[string]string{api.ProjectLabel: e.request.Replay.Project, api.ServiceLabel: e.request.Replay.Service, api.OneoffLabel: "False", api.ContainerNumberLabel: "1", api.ConfigHashLabel: "previous-hash"}
}
func (e *composeEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Api-Version", "1.56")
	w.Header().Set("OSType", "linux")
	path := r.URL.Path
	if strings.HasPrefix(path, "/v1.") {
		_, path, _ = strings.Cut(path[1:], "/")
		path = "/" + path
	}
	reply := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		e.mutations = append(e.mutations, r.Method+" "+path)
	} else {
		e.reads = append(e.reads, path)
	}
	switch {
	case path == "/_ping":
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("OK"))
	case path == "/version":
		reply(map[string]any{"ApiVersion": "1.56", "MinAPIVersion": "1.44", "Version": "29.0.0", "Os": "linux", "Arch": "amd64"})
	case path == "/info":
		reply(map[string]any{"OSType": "linux", "Architecture": "amd64", "ServerVersion": "29.0.0", "Name": "example-engine", "ID": "example"})
	case path == "/containers/json":
		items := []any{}
		if !e.oldRemoved {
			items = append(items, map[string]any{"Id": "old-container", "Names": []string{"/" + e.request.Replay.Project + "-hub-1"}, "Image": e.request.Replay.CurrentReference, "ImageID": "sha256:" + digestHex, "Labels": e.labels(), "State": "running"})
		}
		if e.created {
			state := "created"
			if e.started {
				state = "running"
			}
			items = append(items, map[string]any{"Id": "replacement", "Names": []string{"/" + e.request.Replay.Project + "-hub-1"}, "Image": e.request.TargetReference, "ImageID": "sha256:" + digestHex, "Labels": e.newConfig["Labels"], "State": state})
		}
		reply(items)
	case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
		reply(map[string]any{"Id": "sha256:" + digestHex, "RepoTags": []string{e.request.TargetReference}, "Os": "linux", "Architecture": "amd64", "Config": map[string]any{"Labels": map[string]string{}, "Healthcheck": map[string]any{"Test": []string{"CMD", "/health"}}}})
	case path == "/volumes":
		reply(map[string]any{"Volumes": []any{}, "Warnings": []string{}})
	case path == "/networks":
		reply([]any{map[string]any{"Id": "network", "Name": e.request.Replay.Project + "_default", "Driver": "bridge", "Labels": map[string]string{api.ProjectLabel: e.request.Replay.Project, api.NetworkLabel: "default", api.ConfigHashLabel: "previous-hash"}}})
	case strings.HasPrefix(path, "/networks/"):
		reply(map[string]any{"Id": "network", "Name": e.request.Replay.Project + "_default", "Driver": "bridge", "Labels": map[string]string{api.ProjectLabel: e.request.Replay.Project, api.NetworkLabel: "default", api.ConfigHashLabel: "previous-hash"}, "Containers": map[string]any{}})
	case strings.HasSuffix(path, "/json") && strings.HasPrefix(path, "/containers/"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
		config := map[string]any{"Image": e.request.Replay.CurrentReference, "Labels": e.labels()}
		health := "healthy"
		running := true
		if id == "replacement" {
			config = e.newConfig
			health = "unhealthy"
			running = e.started
		}
		reply(map[string]any{"Id": id, "Name": "/" + e.request.Replay.Project + "-hub-1", "Config": config, "State": map[string]any{"Running": running, "Status": "running", "Health": map[string]any{"Status": health}}, "HostConfig": map[string]any{"NetworkMode": e.request.Replay.Project + "_default"}, "NetworkSettings": map[string]any{"Networks": map[string]any{e.request.Replay.Project + "_default": map[string]any{"NetworkID": "network"}}}, "Mounts": []any{}})
	case path == "/containers/create":
		if err := json.NewDecoder(r.Body).Decode(&e.newConfig); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		e.created = true
		e.images = append(e.images, fmt.Sprint(e.newConfig["Image"]))
		w.WriteHeader(201)
		reply(map[string]any{"Id": "replacement", "Warnings": []string{}})
	case r.Method == http.MethodDelete && path == "/containers/old-container":
		e.oldRemoved = true
		w.WriteHeader(204)
	case r.Method == http.MethodPost && path == "/containers/replacement/start":
		e.started = true
		w.WriteHeader(204)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/containers/"):
		w.WriteHeader(204)
	case path == "/events":
		reply(map[string]any{})
	default:
		http.Error(w, "unhandled synthetic Engine route: "+r.Method+" "+path, 500)
	}
}
func TestEmbeddedComposeFailedHealthDoesNotRollback(t *testing.T) {
	data, request, old := requestFixture(t)
	engine := &composeEngine{request: request}
	receiver := httptest.NewServer(engine)
	defer receiver.Close()
	helper, err := EmbeddedHelper("tcp://" + strings.TrimPrefix(receiver.URL, "http://"))
	require.NoError(t, err)
	runErr := helper.Run(t.Context(), data, request.OpID)
	require.Error(t, runErr)
	engine.mu.Lock()
	defer engine.mu.Unlock()
	require.True(t, engine.started, "real Compose did not start replacement; error=%v reads=%v mutations=%v", runErr, engine.reads, engine.mutations)
	require.Equal(t, []string{request.TargetReference}, engine.images, "health failure must never recreate the old image")
	require.False(t, envBytesEqual(request.Replay.EnvFile, old))
	var result Result
	require.NoError(t, ReadJSON(ResultPath(data, request.OpID), &result))
	require.Equal(t, "failed-health", result.Phase)
}

func TestEmbeddedComposePreEffectFailureRestoresEnv(t *testing.T) {
	data, request, old := requestFixture(t)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "synthetic pre-effect failure", 500) }))
	defer receiver.Close()
	helper, err := EmbeddedHelper("tcp://" + strings.TrimPrefix(receiver.URL, "http://"))
	require.NoError(t, err)
	require.Error(t, helper.Run(t.Context(), data, request.OpID))
	require.True(t, envBytesEqual(request.Replay.EnvFile, old))
	var result Result
	require.NoError(t, ReadJSON(ResultPath(data, request.OpID), &result))
	require.Equal(t, "failed-pre-effect", result.Phase)
}
