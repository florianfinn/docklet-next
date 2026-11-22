// Source: docklet-next/issues/567
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original executor-owned deploy progress observation and behavioural coverage.

package projects

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/stretchr/testify/require"
)

func deployFrame(id, status, detail string) []byte {
	if detail == "" {
		detail = "null"
	}
	return []byte(fmt.Sprintf(`{"id":%q,"status":%q,"progressDetail":%s}`, id, status, detail))
}

func TestDeployPullAggregation(t *testing.T) {
	for _, detail := range []string{`null`, `{}`, `{"current":0}`, `{"current":0,"total":-1}`, `{"current":-1,"total":2}`, `{"current":3,"total":2}`, `{"current":0,"total":9223372036854775808}`} {
		t.Run(detail, func(t *testing.T) {
			var patch map[string]any
			r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
			r.ObservePull("image", deployFrame("abcdef012345", "Downloading", detail))
			require.NotContains(t, patch["deployPull"], "bytesTotal", "invalid totals must hide bytes")
			require.Equal(t, 1, patch["deployPull"].(map[string]any)["layersTotal"])
		})
	}
	var patch map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	r.ObservePull("image", deployFrame("unknown", "Downloading", `{"current":1,"total":2}`))
	require.Nil(t, patch, "unknown layer IDs must not enter counts")
	r.ObservePull("image", deployFrame("abcdef012345", "Downloading", `{"current":40,"total":100}`))
	r.ObservePull("image", deployFrame("abcdef012345", "Download complete", ""))
	r.ObservePull("image", deployFrame("abcdef012345", "Extracting", `{"current":1,"total":200}`))
	r.Flush(true)
	require.EqualValues(t, 100, patch["deployPull"].(map[string]any)["bytesDone"], "extraction must not reset download bytes")
	r.ObservePull("image", deployFrame("abcdef012345", "Pull complete", ""))
	r.ObservePull("image", deployFrame("abcdef012346", "Downloading", `{"current":9223372036854775807,"total":9223372036854775807}`))
	r.Flush(true)
	pull := patch["deployPull"].(map[string]any)
	require.EqualValues(t, math.MaxInt64, pull["bytesTotal"])
	require.Equal(t, 1, pull["layersDone"])
	r.ObservePull("image", deployFrame("abcdef012347", "Waiting", ""))
	r.Flush(true)
	require.NotContains(t, patch["deployPull"], "bytesDone", "every observed layer must have a known total")
}

// Docker reports cached layers with "Already exists" and no detail; the download sum of the other layers stays visible.
func TestDeployPullCachedLayerKeepsBytes(t *testing.T) {
	var patch map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	r.ObservePull("image", deployFrame("abcdef012345", "Already exists", ""))
	r.ObservePull("image", deployFrame("abcdef012346", "Downloading", `{"current":4,"total":12}`))
	r.Flush(true)
	pull := patch["deployPull"].(map[string]any)
	require.EqualValues(t, 4, pull["bytesDone"])
	require.EqualValues(t, 12, pull["bytesTotal"])
	require.Equal(t, 1, pull["layersDone"])
	require.Equal(t, 2, pull["layersTotal"])
}

func TestDeployReporterBoundsAndThrottle(t *testing.T) {
	now := time.Unix(100, 0)
	var patches []map[string]any
	var phases []string
	r := NewDeployReporter(func(s string, _ bool) { phases = append(phases, s) }, func(p map[string]any) { patches = append(patches, p) }, func() time.Time { return now })
	r.Phase("pull")
	for i := 0; i < 10000; i++ {
		r.Phase("pull")
		r.ObservePull(fmt.Sprintf("image-%d", i%100), deployFrame(fmt.Sprintf("%012x", i), "Downloading", `{"current":1,"total":2}`))
	}
	require.Len(t, patches, 1, "metadata must be throttled per run")
	require.Equal(t, []string{"pull"}, phases, "only phase transitions append messages")
	now = now.Add(time.Second)
	r.Flush(false)
	require.Len(t, patches, 2)
	pull := patches[1]["deployPull"].(map[string]any)
	require.Equal(t, 64, pull["images"])
	require.Equal(t, 512, pull["layersTotal"])
	require.NotContains(t, pull, "bytesTotal", "bounds overflow must hide incomplete byte totals")
	r.ObservePull("image-0", deployFrame("000000000000", "Pull complete", ""))
	r.Phase("deploy")
	r.On(api.Resource{ID: "Container app", Text: api.StatusStarting, Status: api.Working})
	require.Len(t, patches, 2)
	r.Flush(true)
	require.Len(t, patches, 3, "final observation bypasses throttle")
	require.Equal(t, []string{"pull", "deploy"}, phases)
	raw, err := json.Marshal(patches[2])
	require.NoError(t, err)
	require.Less(t, len(raw), 1024)
}

func TestDeployResourceObservationsNeverBecomeTrace(t *testing.T) {
	var phases []string
	var patch map[string]any
	r := NewDeployReporter(func(s string, _ bool) { phases = append(phases, s) }, func(p map[string]any) { patch = p }, time.Now)
	r.Phase("pull")
	r.Phase("deploy")
	r.On(api.Resource{ID: "Container app", Text: api.StatusCreating}, api.Resource{ID: "Container worker", Text: api.StatusCreating})
	r.On(api.Resource{ID: "Container app", Text: api.StatusStarting})
	r.On(api.Resource{ID: "Container worker", Text: api.StatusError, Status: api.Error})
	r.Flush(true)
	require.Equal(t, []string{"pull", "deploy"}, phases, "resource work must not create trace completion claims")
	require.Equal(t, map[string]any{"v": 1, "label": "start", "services": 1}, patch["deployActivity"])
}

func TestDeployResourceCountsServicesOnceAcrossReplicas(t *testing.T) {
	var patch map[string]any
	r := NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	r.ConfigureProject(&composetypes.Project{Name: "example", Services: composetypes.Services{"app": {Name: "app"}, "worker": {Name: "worker", ContainerName: "custom-worker"}}})
	r.Phase("deploy")
	r.On(api.Resource{ID: "Container example-app-1", Text: api.StatusStarting}, api.Resource{ID: "Container example-app-2", Text: api.StatusStarting}, api.Resource{ID: "Container custom-worker", Text: api.StatusStarting}, api.Resource{ID: "Volume ignored", Text: api.StatusCreating})
	r.Flush(true)
	require.Equal(t, map[string]any{"v": 1, "label": "start", "services": 2}, patch["deployActivity"])
	r.On(api.Resource{ID: "Container example-app-1", Text: api.StatusStarted, Status: api.Done}, api.Resource{ID: "Container example-app-2", Text: api.StatusStarted, Status: api.Done})
	r.Flush(true)
	require.Equal(t, map[string]any{"v": 1, "label": "start", "services": 1}, patch["deployActivity"], "completed services leave the current observation")
	r.On(api.Resource{ID: "Container custom-worker", Text: api.StatusStarted, Status: api.Done})
	r.Flush(true)
	require.Nil(t, patch["deployActivity"], "completed resource activity must not remain current")
}
