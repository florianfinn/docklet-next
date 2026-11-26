// Source: docklet-next/issues/655
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original per-service deploy stage observation and behavioural coverage.

package projects

import (
	"fmt"
	"testing"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/stretchr/testify/require"
)

func serviceRows(t *testing.T, patch map[string]any) map[string]map[string]any {
	t.Helper()
	rows := map[string]map[string]any{}
	for _, row := range patch["deployServices"].(map[string]any)["services"].([]map[string]any) {
		rows[row["name"].(string)] = row
	}
	return rows
}

func stageProject() *composetypes.Project {
	return &composetypes.Project{Name: "example", Services: composetypes.Services{
		"app":     {Name: "app", Image: "example/app:1", HealthCheck: &composetypes.HealthCheckConfig{Test: []string{"CMD", "true"}}},
		"worker":  {Name: "worker", Image: "example/worker:1", HealthCheck: &composetypes.HealthCheckConfig{Disable: true}},
		"builder": {Name: "builder", Image: "example-builder", Build: &composetypes.BuildConfig{Context: "."}},
	}}
}

func TestDeployServiceRowsFollowStagesPerService(t *testing.T) {
	var patch map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	r.ConfigureProject(stageProject())
	r.Phase("pull")
	r.ObservePull("example/app:1", deployFrame("abcdef012345", "Already exists", ""))
	r.ObservePull("example/app:1", deployFrame("abcdef012346", "Downloading", `{"current":30,"total":120}`))
	r.Build("builder", deployStageRunning)
	r.Flush(true)
	rows := serviceRows(t, patch)
	require.Equal(t, map[string]any{"layersDone": 1, "layersTotal": 2, "bytesDone": int64(30), "bytesTotal": int64(120)}, rows["app"]["pull"])
	require.Equal(t, true, rows["app"]["check"])
	require.Equal(t, false, rows["worker"]["check"], "a disabled healthcheck is known to be absent")
	require.NotContains(t, rows["builder"], "check", "without a declaration the image may still define a healthcheck")
	require.NotContains(t, rows["worker"], "pull", "an image without observed layers claims no pull")
	require.Equal(t, deployStageRunning, rows["builder"]["build"])

	r.Build("builder", deployStageDone)
	r.Phase("deploy")
	r.On(
		api.Resource{ID: "Container example-app-1", Text: api.StatusCreating, Status: api.Working},
		api.Resource{ID: "Container example-app-1", Text: api.StatusCreated, Status: api.Done},
		api.Resource{ID: "Container example-app-1", Text: api.StatusStarting, Status: api.Working},
		api.Resource{ID: "Container example-app-1", Text: api.StatusStarted, Status: api.Done},
		api.Resource{ID: "Container example-app-1", Text: api.StatusWaiting, Status: api.Working},
		api.Resource{ID: "Container example-worker-1", Text: api.StatusRunning, Status: api.Done},
		api.Resource{ID: "Container example-worker-2", Text: api.StatusStarting, Status: api.Working},
		api.Resource{ID: "Container example-builder-1", Text: "Recreate", Status: api.Working},
		api.Resource{ID: "Container example-builder-1", Text: "Recreated", Status: api.Done},
		api.Resource{ID: "Container example-builder-1", Text: "boom", Status: api.Error},
		api.Resource{ID: "Network example_default", Text: api.StatusCreating, Status: api.Working},
	)
	r.Flush(true)
	rows = serviceRows(t, patch)
	require.Equal(t, deployStageDone, rows["app"]["create"])
	require.Equal(t, deployStageDone, rows["app"]["start"])
	require.Equal(t, deployStageRunning, rows["app"]["health"])
	require.Equal(t, deployStageRunning, rows["worker"]["start"], "a replica still starting keeps the service running")
	require.NotContains(t, rows["worker"], "create", "an unchanged container reports no create")
	require.Equal(t, deployStageDone, rows["builder"]["build"])
	require.Equal(t, deployStageError, rows["builder"]["start"], "an error after create belongs to start")

	r.On(api.Resource{ID: "Container example-app-1", Text: api.StatusHealthy, Status: api.Done}, api.Resource{ID: "Container example-worker-2", Text: api.StatusError, Status: api.Error})
	r.Flush(true)
	rows = serviceRows(t, patch)
	require.Equal(t, deployStageDone, rows["app"]["health"])
	require.Equal(t, deployStageError, rows["worker"]["start"], "a failing replica wins over a started one")
}

func TestDeployServiceRowsBoundsAndRebinding(t *testing.T) {
	var patch map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	r.Phase("deploy")
	r.On(api.Resource{ID: "Container app", Text: api.StatusStarting, Status: api.Working})
	r.Flush(true)
	require.NotContains(t, patch, "deployServices", "rows come only from the Compose model")

	services := composetypes.Services{}
	for i := range 70 {
		name := fmt.Sprintf("svc-%02d", i)
		services[name] = composetypes.ServiceConfig{Name: name}
	}
	r.ConfigureProject(&composetypes.Project{Name: "example", Services: services})
	r.ConfigureProject(&composetypes.Project{Name: "example", Services: services})
	r.Flush(true)
	result := patch["deployServices"].(map[string]any)
	rows := result["services"].([]map[string]any)
	require.Len(t, rows, deployServiceRowLimit)
	require.Equal(t, true, result["truncated"])
	require.Equal(t, "svc-00", rows[0]["name"], "rows are ordered by name")
	require.Len(t, r.services, 70, "rebinding replaces instead of appending")

	project := stageProject()
	r.ConfigureProject(project)
	builder := project.Services["builder"]
	builder.Image = "example-builder:resolved"
	project.Services["builder"] = builder
	r.ConfigureProject(project)
	r.Phase("pull")
	r.ObservePull("example-builder:resolved", deployFrame("abcdef012345", "Pull complete", ""))
	r.Flush(true)
	require.NotContains(t, patch["deployServices"], "truncated")
	require.Equal(t, 1, serviceRows(t, patch)["builder"]["pull"].(map[string]any)["layersDone"], "rebinding follows the resolved image")
	r.Phase("deploy")
	r.On(api.Resource{ID: "Container example-app-1", Text: api.StatusRunning, Status: api.Done})
	r.Flush(true)
	require.Equal(t, deployStageDone, serviceRows(t, patch)["app"]["start"], "an unchanged running container counts as started")
}

// Update runs pull digest pins whose references differ from the Compose image.
func TestDeployServiceRowsFollowBoundPins(t *testing.T) {
	var patch map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	pin := "registry.example.com/app@sha256:" + fmt.Sprintf("%064d", 1)
	r.BindImage("app", pin)
	r.ObservePull(pin, deployFrame("abcdef012345", "Downloading", `{"current":5,"total":10}`))
	r.Flush(true)
	require.Equal(t, map[string]any{"layersDone": 0, "layersTotal": 1, "bytesDone": int64(5), "bytesTotal": int64(10)}, serviceRows(t, patch)["app"]["pull"], "a bound pin yields a row before the Compose model is known")

	project := stageProject()
	r.ConfigureProject(project)
	r.Flush(true)
	rows := serviceRows(t, patch)
	require.Len(t, rows, 3)
	require.Equal(t, 1, rows["app"]["pull"].(map[string]any)["layersTotal"], "the binding survives rebinding to the Compose image")
}

func TestDeployServiceBindingKeepsFirstPullUnthrottled(t *testing.T) {
	now := time.Unix(100, 0)
	var patches []map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patches = append(patches, p) }, func() time.Time { return now })
	r.ConfigureProject(stageProject())
	r.BindImage("app", "example/app:1")
	r.ObservePull("example/app:1", deployFrame("abcdef012345", "Downloading", `{"current":1,"total":2}`))
	require.Len(t, patches, 1, "binding must not consume the first patch")
	require.NotNil(t, patches[0]["deployPull"])
	require.Contains(t, patches[0], "deployServices")
}

// Compose 5.6 reports a failed create under its structured replica ID, not the container name.
func TestDeployServiceRowsCreateErrorAndPreparedBoundary(t *testing.T) {
	var patch map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	r.ConfigureProject(stageProject())
	r.Flush(true)
	require.NotContains(t, patch["deployServices"], "prepared", "preparation is still open")
	r.Phase("deploy")
	r.On(
		api.Resource{ID: "Container example-app-1", Text: api.StatusCreating, Status: api.Working},
		api.Resource{ID: "service:app:1", Text: "create failed", Status: api.Error},
		api.Resource{ID: "service:unknown:1", Text: "ignored", Status: api.Error},
		api.Resource{ID: "service:worker:x", Text: "ignored", Status: api.Error},
	)
	r.Flush(true)
	require.Equal(t, true, patch["deployServices"].(map[string]any)["prepared"])
	rows := serviceRows(t, patch)
	require.Equal(t, deployStageError, rows["app"]["create"], "the structured create error reaches its service")
	require.NotContains(t, rows["worker"], "create")
	require.Len(t, rows, 3, "unknown services gain no rows")
}

func TestDeployServiceFirstPullSurvivesEarlierRowTick(t *testing.T) {
	now := time.Unix(100, 0)
	var patches []map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patches = append(patches, p) }, func() time.Time { return now })
	r.ConfigureProject(stageProject())
	r.Flush(false)
	require.Len(t, patches, 1, "the periodic tick publishes the bound rows")
	now = now.Add(100 * time.Millisecond)
	r.ObservePull("example/app:1", deployFrame("abcdef012345", "Downloading", `{"current":1,"total":2}`))
	require.Len(t, patches, 2, "the first pull frame is not throttled by the row tick")
	require.NotNil(t, patches[1]["deployPull"])
	r.ObservePull("example/app:1", deployFrame("abcdef012345", "Downloading", `{"current":2,"total":2}`))
	require.Len(t, patches, 2, "later frames keep the throttle")
}

func TestDeployServiceBindingCapIsStable(t *testing.T) {
	services := composetypes.Services{}
	for i := range 600 {
		name := fmt.Sprintf("svc-%03d", i)
		services[name] = composetypes.ServiceConfig{Name: name}
	}
	r := NewDeployReporter(func(string, bool) {}, func(map[string]any) {}, time.Now)
	for range 5 {
		r.ConfigureProject(&composetypes.Project{Name: "example", Services: services})
		require.Len(t, r.services, 512)
		require.Equal(t, "svc-000", r.services[0].name)
		require.Equal(t, "svc-511", r.services[511].name, "the cap keeps the first services by name")
	}
}

// Compose 5.6 waits on service_completed_successfully with Waiting, then Exited on a zero exit code.
func TestDeployServiceRowsRecordCompletedOneShot(t *testing.T) {
	var patch map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	r.ConfigureProject(stageProject())
	r.Phase("deploy")
	r.On(
		api.Resource{ID: "Container example-builder-1", Text: api.StatusStarted, Status: api.Done},
		api.Resource{ID: "Container example-builder-1", Text: api.StatusWaiting, Status: api.Working},
		api.Resource{ID: "Container example-builder-1", Text: api.StatusExited, Status: api.Done},
		api.Resource{ID: "Container example-worker-1", Text: api.StatusWaiting, Status: api.Working},
		api.Resource{ID: "Container example-worker-1", Text: "dependency failed", Status: api.Error},
	)
	r.Flush(true)
	rows := serviceRows(t, patch)
	require.Equal(t, deployStageDone, rows["builder"]["health"])
	require.Equal(t, true, rows["builder"]["completed"], "a successful one-shot exit is a confirmed completion")
	require.Equal(t, deployStageError, rows["worker"]["health"], "a failed wait stays an error even with a disabled healthcheck")
	require.NotContains(t, rows["worker"], "completed")
}

func TestDeployServicePreparedBoundaryIsExplicit(t *testing.T) {
	now := time.Unix(100, 0)
	var patch map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, func() time.Time { return now })
	r.ConfigureProject(stageProject())
	r.Flush(false)
	require.NotContains(t, patch["deployServices"], "prepared")
	now = now.Add(100 * time.Millisecond)
	r.Phase("deploy")
	now = now.Add(time.Second)
	r.Flush(false)
	require.Equal(t, true, patch["deployServices"].(map[string]any)["prepared"], "the boundary is published after a tick drained the rows")

	failed := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	failed.ConfigureProject(stageProject())
	failed.PrepareFailed()
	failed.Flush(true)
	require.NotContains(t, patch["deployServices"], "prepared", "a failed preparation never claims its pulls were not needed")
}
