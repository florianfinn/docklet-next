// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original stateful Docker HTTP producer for recreate transport and crash tests.

package activity_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type recreateObserved struct {
	ID, Name            string
	Labels              map[string]string
	Running, AutoRemove bool
	Created             string
	Environment         []string
}
type recreateHTTP struct {
	t                *testing.T
	mu               sync.Mutex
	containers       map[string]recreateObserved
	effects          map[string]int
	fail             map[string]bool
	hold             string
	entered, release chan struct{}
	once             sync.Once
}

const recreateNewID = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

func newRecreateHTTP(t *testing.T) *recreateHTTP {
	return &recreateHTTP{t: t, containers: map[string]recreateObserved{runtimeContainerID: {ID: runtimeContainerID, Name: "app", Labels: map[string]string{}, Running: true, Created: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)}}, effects: map[string]int{}, fail: map[string]bool{}}
}
func (d *recreateHTTP) copy(id string) (recreateObserved, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	value, ok := d.containers[id]
	return value, ok
}
func (d *recreateHTTP) counts() map[string]int {
	d.mu.Lock()
	defer d.mu.Unlock()
	copy := map[string]int{}
	for k, v := range d.effects {
		copy[k] = v
	}
	return copy
}
func (d *recreateHTTP) lookup(id string) (recreateObserved, bool) {
	if value, ok := d.containers[id]; ok {
		return value, true
	}
	for _, value := range d.containers {
		if value.Name == id {
			return value, true
		}
	}
	return recreateObserved{}, false
}
func (d *recreateHTTP) serve(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create") {
		d.mu.Lock()
		d.effects["pull"]++
		d.mu.Unlock()
		return false
	}
	marker := strings.Index(r.URL.Path, "/containers/")
	if marker < 0 {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path[marker:], "/containers/"), "/")
	w.Header().Set("Content-Type", "application/json")
	if len(parts) == 1 && parts[0] == "json" && r.Method == http.MethodGet {
		_, _ = w.Write([]byte("[]"))
		return true
	}
	if len(parts) == 2 && parts[1] == "json" && r.Method == http.MethodGet {
		d.mu.Lock()
		value, ok := d.lookup(parts[0])
		d.mu.Unlock()
		if !ok {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
			return true
		}
		state := "exited"
		if value.Running {
			state = "running"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": value.ID, "Name": "/" + value.Name, "Created": value.Created, "Config": map[string]any{"Image": "registry.example.com/app:1.0.0", "Labels": value.Labels, "Env": value.Environment}, "State": map[string]any{"Running": value.Running, "Status": state}, "HostConfig": map[string]any{"AutoRemove": value.AutoRemove}, "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
		return true
	}
	operation := ""
	if len(parts) == 1 && parts[0] == "create" && r.Method == http.MethodPost {
		operation = "create"
	} else if len(parts) == 2 && r.Method == http.MethodPost {
		operation = parts[1]
	} else if len(parts) == 1 && r.Method == http.MethodDelete {
		operation = "remove"
	}
	if operation == "" {
		return false
	}
	d.mu.Lock()
	d.effects[operation]++
	fail := d.fail[operation]
	hold := d.hold == operation
	d.mu.Unlock()
	if hold {
		d.once.Do(func() { close(d.entered) })
		<-d.release
	}
	if fail {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"message":"synthetic effect refusal"}`))
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if operation == "create" {
		var body struct {
			Labels     map[string]string
			Env        []string
			HostConfig struct{ AutoRemove bool }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			d.t.Errorf("decode create: %v", err)
			w.WriteHeader(400)
			return true
		}
		d.containers[recreateNewID] = recreateObserved{ID: recreateNewID, Name: r.URL.Query().Get("name"), Labels: body.Labels, Environment: body.Env, Created: time.Now().UTC().Format(time.RFC3339Nano), AutoRemove: body.HostConfig.AutoRemove}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": recreateNewID, "Warnings": []string{}})
		return true
	}
	value, ok := d.lookup(parts[0])
	if !ok {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
		return true
	}
	switch operation {
	case "rename":
		value.Name = r.URL.Query().Get("name")
		d.containers[value.ID] = value
	case "start":
		value.Running = true
		d.containers[value.ID] = value
	case "stop":
		value.Running = false
		if value.AutoRemove {
			delete(d.containers, value.ID)
		} else {
			d.containers[value.ID] = value
		}
	case "remove":
		delete(d.containers, value.ID)
	default:
		w.WriteHeader(500)
		return true
	}
	w.WriteHeader(204)
	return true
}
