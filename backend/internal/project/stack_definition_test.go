// Source: docklet-next/issues/317
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original actual-list dependency edits, invalid repair and live-model isolation proof.

package project

import (
	"fmt"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	dto "github.com/florianfinn/docklet-next/types/project"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStoppedStackListingTracksDependenciesAndRepair(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", "include:\n  - included.yaml\nservices:\n  app:\n    image: registry.example.com/app:${TAG}\n")
	server := newProjectRuntimeDockerServer(t, nil)
	svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc.initChildren(nil, nil)
	var version int64
	write := func(name, body string) {
		version++
		path := filepath.Join(row.Path, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0600))
		stamp := time.Unix(1800000000+version, 0)
		require.NoError(t, os.Chtimes(path, stamp, stamp))
	}
	write(".env", "TAG=first\n")
	write("included.yaml", "services:\n  worker:\n    image: registry.example.com/worker:1\n")
	list := func() dto.Details {
		for _, sort := range []string{"name", "status"} {
			rows, _, err := svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1, Sort: sort})
			require.NoError(t, err)
			require.Len(t, rows, 1)
			if sort == "status" {
				return rows[0]
			}
		}
		panic("missing row")
	}
	check := func(count int, image string) {
		got := list()
		require.Empty(t, got.DefinitionHint)
		require.Len(t, got.RuntimeServices, count)
		require.Equal(t, dto.StatusStopped, got.Status)
		for _, s := range got.RuntimeServices {
			require.Equal(t, dto.ServiceNotCreated, s.Status)
			require.Empty(t, s.ContainerID)
			require.Empty(t, s.ContainerName)
			if s.Name == "app" {
				require.Equal(t, image, s.Image)
			}
		}
	}
	check(2, "registry.example.com/app:first")
	write("included.yaml", "services:\n  worker:\n    image: registry.example.com/worker:2\n  queue:\n    image: registry.example.com/queue:1\n")
	check(3, "registry.example.com/app:first")
	write(".env", "TAG=second\n")
	check(3, "registry.example.com/app:second")
	write("compose.override.yaml", "services:\n  app:\n    image: registry.example.com/app:override\n  sidecar:\n    image: registry.example.com/sidecar:1\n")
	check(4, "registry.example.com/app:override")
	write("included.yaml", "services: [invalid\n")
	broken := list()
	require.Equal(t, "definition-invalid", broken.DefinitionHint)
	require.Empty(t, broken.RuntimeServices)
	write("included.yaml", "services:\n  worker:\n    image: registry.example.com/worker:repaired\n")
	check(3, "registry.example.com/app:override")
}

func TestLiveStackListingDoesNotResolveDefinitionServices(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", "services: [invalid\n")
	server := newProjectRuntimeDockerServer(t, []container.Summary{{ID: "example-container", Names: []string{"/alpha-app-1"}, Image: "registry.example.com/app:1", State: container.StateRunning, Labels: map[string]string{"com.docker.compose.project": "alpha", "com.docker.compose.service": "app"}}})
	svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc.initChildren(nil, nil)
	rows, _, err := svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, row.ID, rows[0].ID)
	require.Empty(t, rows[0].DefinitionHint)
	require.Len(t, rows[0].RuntimeServices, 1)
	require.Equal(t, "example-container", rows[0].RuntimeServices[0].ContainerID)
	require.Equal(t, "running", rows[0].RuntimeServices[0].Status)
}

// The cache probe holds the first four reads at the actual resolution seam.
// Every page row has its own output slot; shared Compose caches remain isolated.
type heldDefinitionCache struct {
	entered chan struct{}
	release chan struct{}
	active  atomic.Int32
	maximum atomic.Int32
	model   *composetypes.Project
}

func (c *heldDefinitionCache) Get(string, string) (*composetypes.Project, bool) {
	n := c.active.Add(1)
	for previous := c.maximum.Load(); n > previous; previous = c.maximum.Load() {
		if c.maximum.CompareAndSwap(previous, n) {
			break
		}
	}
	c.entered <- struct{}{}
	<-c.release
	c.active.Add(-1)
	return c.model, true
}
func (*heldDefinitionCache) Set(string, string, string, string, string, []string, []string, *composetypes.Project) error {
	return nil
}
func (*heldDefinitionCache) Invalidate(string) {}
func TestDefinitionServicesResolutionBoundedToFour(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	var records []Project
	var rows []dto.Details
	for i := range 8 {
		record := runtimeIdentityProject(t, svc, root, fmt.Sprintf("example-%d", i), fmt.Sprintf("stack-%d", i), "services:\n  app:\n    image: registry.example.com/app:1\n")
		records = append(records, *record)
		rows = append(rows, dto.Details{ID: record.ID, Status: dto.StatusStopped})
	}
	cache := &heldDefinitionCache{entered: make(chan struct{}, 8), release: make(chan struct{}), model: &composetypes.Project{Services: composetypes.Services{"app": {Name: "app", Image: "registry.example.com/app:1"}}}}
	unblock := sync.OnceFunc(func() { close(cache.release) })
	defer unblock()
	svc.parsedCompose = cache
	done := make(chan struct{})
	env := svc.newProjectMetadataEnv(t.Context(), records)
	go func() { defer close(done); svc.fillDefinitionServices(t.Context(), records, rows, env) }()
	for range 4 {
		select {
		case <-cache.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("four resolution workers did not enter")
		}
	}
	unblock()
	<-done
	require.EqualValues(t, 4, cache.maximum.Load())
	for _, row := range rows {
		require.Len(t, row.RuntimeServices, 1)
		require.Equal(t, dto.ServiceNotCreated, row.RuntimeServices[0].Status)
	}
}
